package agentbbolt

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/launcher"
	"go.etcd.io/bbolt"
)

const (
	backupInitialDelay = 10 * time.Minute
	backupInterval     = 1 * time.Hour
)

// databaseBackupMaintainer periodically takes backups of launcher.db.
type databaseBackupMaintainer struct {
	knapsack    types.Knapsack
	slogger     *slog.Logger
	interrupt   chan struct{}
	interrupted bool
}

func NewDatabaseBackupMaintainer(k types.Knapsack) *databaseBackupMaintainer {
	return &databaseBackupMaintainer{
		knapsack:  k,
		slogger:   k.Slogger().With("component", "database_backup_maintainer"),
		interrupt: make(chan struct{}, 1),
	}
}

func (d *databaseBackupMaintainer) Execute() error {
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

func (d *databaseBackupMaintainer) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if d.interrupted {
		return
	}
	d.interrupted = true

	d.interrupt <- struct{}{}
}

func (d *databaseBackupMaintainer) backupDb() error {
	// Take backup -- it's fine to just overwrite previous backups
	backupLocation := BackupLauncherDbLocation(d.knapsack.RootDirectory())
	if err := d.knapsack.BboltDB().View(func(tx *bbolt.Tx) error {
		return tx.CopyFile(backupLocation, 0600)
	}); err != nil {
		return fmt.Errorf("backing up database: %w", err)
	}

	// Confirm file exists and is nonempty
	if exists, err := launcher.NonEmptyFileExists(backupLocation); !exists {
		return fmt.Errorf("backup succeeded, but nonempty file does not exist at %s", backupLocation)
	} else if err != nil {
		return fmt.Errorf("backup succeeded, but error checking if file was created at %s: %w", backupLocation, err)
	}

	// Log success
	d.slogger.Log(context.TODO(), slog.LevelDebug,
		"took backup",
		"backup_location", backupLocation,
	)

	return nil
}

// UseBackupDbIfNeeded falls back to the backup database IFF the original database does not exist
// and the backup does. In this case, it renames the backup database to the expected filename
// launcher.db.
func UseBackupDbIfNeeded(rootDir string, slogger *slog.Logger) {
	// Check first to see if the regular database exists
	originalDbLocation := LauncherDbLocation(rootDir)
	if originalDbExists, err := launcher.NonEmptyFileExists(originalDbLocation); originalDbExists {
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
	backupDbExists, err := launcher.NonEmptyFileExists(backupLocation)
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
