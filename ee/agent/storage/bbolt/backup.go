package agentbbolt

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
	"go.etcd.io/bbolt"
)

const (
	backupInitialDelay         = 10 * time.Minute
	backupInterval             = 1 * time.Hour
	numberOfOldBackupsToRetain = 3
)

// databaseBackupSaver periodically takes backups of launcher.db.
type databaseBackupSaver struct {
	knapsack    types.Knapsack
	slogger     *slog.Logger
	interrupt   chan struct{}
	interrupted bool
}

func NewDatabaseBackupSaver(k types.Knapsack) *databaseBackupSaver {
	return &databaseBackupSaver{
		knapsack:  k,
		slogger:   k.Slogger().With("component", "database_backup_saver"),
		interrupt: make(chan struct{}, 1),
	}
}

func (d *databaseBackupSaver) Execute() error {
	// Wait a little bit after startup before taking first backup, to allow for enrollment
	select {
	case <-d.interrupt:
		d.slogger.Log(context.TODO(), slog.LevelDebug,
			"received external interrupt during initial delay, stopping",
		)
		return nil
	case <-time.After(backupInitialDelay):
		break
	}

	// Take periodic backups
	ticker := time.NewTicker(backupInterval)
	defer ticker.Stop()
	for {
		if err := d.backupDb(); err != nil {
			d.slogger.Log(context.TODO(), slog.LevelWarn,
				"could not perform periodic database backup",
				"err", err,
			)
		}

		select {
		case <-ticker.C:
			continue
		case <-d.interrupt:
			d.slogger.Log(context.TODO(), slog.LevelDebug,
				"interrupt received, exiting execute loop",
			)
			return nil
		}
	}
}

func (d *databaseBackupSaver) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if d.interrupted {
		return
	}
	d.interrupted = true

	d.interrupt <- struct{}{}
}

func (d *databaseBackupSaver) backupDb() error {
	// Take backup in temporary location
	backupLocation := BackupLauncherDbLocation(d.knapsack.RootDirectory())
	tempBackupLocation := fmt.Sprintf("%s.tmp", backupLocation)
	defer func() {
		// In case we errored out when taking the backup, clean up the temp file
		_ = os.Remove(tempBackupLocation)
	}()

	if err := d.knapsack.BboltDB().View(func(tx *bbolt.Tx) error {
		return tx.CopyFile(tempBackupLocation, 0600)
	}); err != nil {
		return fmt.Errorf("backing up database: %w", err)
	}

	// Confirm file exists and is nonempty
	if exists, err := nonEmptyFileExists(tempBackupLocation); !exists {
		return fmt.Errorf("backup succeeded, but nonempty file does not exist at %s", tempBackupLocation)
	} else if err != nil {
		return fmt.Errorf("backup succeeded, but error checking if file was created at %s: %w", tempBackupLocation, err)
	}

	// Perform rotation of older backups so we can move this backup to `backupLocation`
	if err := d.rotate(); err != nil {
		return fmt.Errorf("backup succeeded, but rotation did not: %w", err)
	}

	if err := os.Rename(tempBackupLocation, backupLocation); err != nil {
		return fmt.Errorf("renaming temp backup %s to %s after rotation: %w", tempBackupLocation, backupLocation, err)
	}

	// Log success
	d.slogger.Log(context.TODO(), slog.LevelDebug,
		"took backup",
		"backup_location", backupLocation,
	)

	return nil
}

func (d *databaseBackupSaver) rotate() error {
	baseBackupPath := BackupLauncherDbLocation(d.knapsack.RootDirectory())

	for i := numberOfOldBackupsToRetain; i > 0; i -= 1 {
		currentBackupPath := fmt.Sprintf("%s.%d", baseBackupPath, i)

		// This backup doesn't exist yet -- skip it
		if _, err := os.Stat(currentBackupPath); err != nil && os.IsNotExist(err) {
			continue
		}

		// If is the oldest backup, delete it so we can rotate a new one into its place
		if i == numberOfOldBackupsToRetain {
			if err := os.Remove(currentBackupPath); err != nil {
				return fmt.Errorf("removing oldest backup %s during rotation: %w", currentBackupPath, err)
			}
			continue
		}

		// Rename from launcher.db.bak.<n> to launcher.db.bak.<n+1>
		olderBackupPath := fmt.Sprintf("%s.%d", baseBackupPath, i+1)
		if err := os.Rename(currentBackupPath, olderBackupPath); err != nil {
			return fmt.Errorf("renaming %s to %s during rotation: %w", currentBackupPath, olderBackupPath, err)
		}
	}

	if err := os.Rename(baseBackupPath, fmt.Sprintf("%s.1", baseBackupPath)); err != nil {
		return fmt.Errorf("rotating %s: %w", baseBackupPath, err)
	}

	return nil
}

// UseBackupDbIfNeeded falls back to the backup database IFF the original database does not exist
// and the backup does. In this case, it renames the backup database to the expected filename
// launcher.db.
func UseBackupDbIfNeeded(rootDir string, slogger *slog.Logger) {
	// Check first to see if the regular database exists
	originalDbLocation := LauncherDbLocation(rootDir)
	if originalDbExists, err := nonEmptyFileExists(originalDbLocation); originalDbExists {
		// DB exists -- we should use that
		slogger.Log(context.TODO(), slog.LevelDebug,
			"launcher.db exists, no need to use backup",
			"db_location", originalDbLocation,
		)
		return
	} else if err != nil {
		// Can't determine whether the db exists -- err on the side of not replacing it
		slogger.Log(context.TODO(), slog.LevelWarn,
			"could not determine whether original launcher db exists, not going to use backup",
			"err", err,
		)
		return
	}

	// Launcher DB doesn't exist -- check to see if the backup does
	backupLocation := BackupLauncherDbLocation(rootDir)
	backupDbExists, err := nonEmptyFileExists(backupLocation)
	if !backupDbExists {
		// Backup DB doesn't exist either -- this is likely a fresh install.
		// Nothing to do here; launcher should create a new DB.
		slogger.Log(context.TODO(), slog.LevelInfo,
			"both launcher db and backup db do not exist -- likely a fresh install",
		)
		return
	}
	if err != nil {
		// Couldn't determine if the backup DB exists -- let launcher create a new DB instead.
		slogger.Log(context.TODO(), slog.LevelWarn,
			"could not determine whether backup launcher db exists, not going to use backup",
			"err", err,
		)
		return
	}

	// The backup database exists, and the original one does not. Rename the backup
	// to the original so we can use it.
	if err := os.Rename(backupLocation, originalDbLocation); err != nil {
		slogger.Log(context.TODO(), slog.LevelWarn,
			"could not rename backup db",
			"backup_location", backupLocation,
			"original_location", originalDbLocation,
			"err", err,
		)
		return
	}
	slogger.Log(context.TODO(), slog.LevelInfo,
		"original db does not exist and backup does -- using backup db",
		"backup_location", backupLocation,
		"original_location", originalDbLocation,
	)
}

func LauncherDbLocation(rootDir string) string {
	return filepath.Join(rootDir, "launcher.db")
}

func BackupLauncherDbLocation(rootDir string) string {
	return filepath.Join(rootDir, "launcher.db.bak")
}

func nonEmptyFileExists(path string) (bool, error) {
	fileInfo, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return fileInfo.Size() > 0, nil
}
