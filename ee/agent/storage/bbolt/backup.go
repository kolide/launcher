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
	snapshotInitialDelay = 10 * time.Minute
	snapshotInterval     = 1 * time.Hour
)

// A photographer takes snapshots.
// TODO RM - A better name.
type photographer struct {
	knapsack    types.Knapsack
	slogger     *slog.Logger
	interrupt   chan struct{}
	interrupted bool
}

func NewDatabasePhotographer(k types.Knapsack) *photographer {
	return &photographer{
		knapsack:  k,
		slogger:   k.Slogger().With("component", "database_photographer"),
		interrupt: make(chan struct{}, 1),
	}
}

func (p *photographer) Execute() error {
	// Wait a little bit after startup before taking first snapshot, to allow for enrollment
	select {
	case <-p.interrupt:
		p.slogger.Log(context.TODO(), slog.LevelDebug,
			"received external interrupt during initial delay, stopping",
		)
		return nil
	case <-time.After(snapshotInitialDelay):
		break
	}

	// Take periodic snapshots
	ticker := time.NewTicker(snapshotInterval)
	defer ticker.Stop()
	for {
		if err := p.backupDb(); err != nil {
			p.slogger.Log(context.TODO(), slog.LevelWarn,
				"could not perform periodic database backup",
				"err", err,
			)
		}

		select {
		case <-ticker.C:
			continue
		case <-p.interrupt:
			p.slogger.Log(context.TODO(), slog.LevelDebug,
				"interrupt received, exiting execute loop",
			)
			return nil
		}
	}
}

func (p *photographer) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if p.interrupted {
		return
	}
	p.interrupted = true

	p.interrupt <- struct{}{}
}

func (p *photographer) backupDb() error {
	// Take backup -- it's fine to just overwrite previous backups
	backupLocation := BackupLauncherDbLocation(p.knapsack.RootDirectory())
	if err := p.knapsack.BboltDB().View(func(tx *bbolt.Tx) error {
		return tx.CopyFile(backupLocation, 0600)
	}); err != nil {
		return fmt.Errorf("backing up database: %w", err)
	}

	// Confirm file exists and is nonempty
	fileInfo, err := os.Stat(backupLocation)
	if os.IsNotExist(err) {
		return fmt.Errorf("backup succeeded, but no file at backup location %s", backupLocation)
	}
	if err != nil {
		return fmt.Errorf("checking %s exists after taking backup: %w", backupLocation, err)
	}
	if fileInfo.Size() <= 0 {
		return fmt.Errorf("backup succeeded, but backup database at %s is empty", backupLocation)
	}

	// Log success
	p.slogger.Log(context.TODO(), slog.LevelDebug,
		"took backup",
		"backup_location", backupLocation,
	)

	return nil
}

func LauncherDbLocation(rootDir string) string {
	return filepath.Join(rootDir, "launcher.db")
}

func BackupLauncherDbLocation(rootDir string) string {
	return filepath.Join(rootDir, "launcher.db.bak")
}
