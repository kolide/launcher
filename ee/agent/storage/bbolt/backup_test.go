package agentbbolt

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

func TestUseBackupDbIfNeeded(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name                string
		originalDbExists    bool
		backupDbExists      bool
		shouldPerformRename bool
	}{
		{
			name:                "original exists, backup exists, should use original",
			originalDbExists:    true,
			backupDbExists:      true,
			shouldPerformRename: false,
		},
		{
			name:                "original exists, backup does not exist, should use original",
			originalDbExists:    true,
			backupDbExists:      false,
			shouldPerformRename: false,
		},
		{
			name:                "original does not exist, backup exists, should use backup",
			originalDbExists:    false,
			backupDbExists:      true,
			shouldPerformRename: true,
		},
		{
			name:                "original does not exist, backup does not exist, should use (new) original",
			originalDbExists:    false,
			backupDbExists:      false,
			shouldPerformRename: false,
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Set up test databases
			tempRootDir := t.TempDir()
			originalDbFileLocation := LauncherDbLocation(tempRootDir)
			backupDbFileLocation := BackupLauncherDbLocation(tempRootDir)
			if tt.originalDbExists {
				createNonEmptyBboltDb(t, originalDbFileLocation)
			}
			if tt.backupDbExists {
				createNonEmptyBboltDb(t, backupDbFileLocation)
			}

			// Ask agentbbolt to use the backup database if the original one isn't present
			UseBackupDbIfNeeded(tempRootDir, multislogger.NewNopLogger())

			// Check to make sure appropriate action was taken
			if tt.shouldPerformRename {
				// The backup database should no longer exist
				_, err := os.Stat(backupDbFileLocation)
				require.Error(t, err, "should not be able to stat launcher.db.bak since it should have been renamed")
				require.True(t, os.IsNotExist(err), "checking that launcher.db.bak does not exist, and error is not ErrNotExist")

				// The original database should exist
				_, err = os.Stat(originalDbFileLocation)
				require.NoError(t, err, "checking if launcher.db exists")
			} else {
				// No rename, so we should be in the same state we started in
				_, err := os.Stat(originalDbFileLocation)
				if tt.originalDbExists {
					require.NoError(t, err, "checking if launcher.db exists")
				} else {
					// launcher.db didn't exist before, it shouldn't exist now
					require.True(t, os.IsNotExist(err), "checking that launcher.db does not exist, and error is not ErrNotExist")
				}

				_, err = os.Stat(backupDbFileLocation)
				if tt.backupDbExists {
					require.NoError(t, err, "checking if launcher.db.bak exists")
				} else {
					// launcher.db.bak didn't exist before, it shouldn't exist now
					require.True(t, os.IsNotExist(err), "checking that launcher.db.bak does not exist, and error is not ErrNotExist")
				}
			}
		})
	}
}

func createNonEmptyBboltDb(t *testing.T, dbFileLocation string) time.Time {
	boltOptions := &bbolt.Options{Timeout: time.Duration(5) * time.Second}
	db, err := bbolt.Open(dbFileLocation, 0600, boltOptions)
	require.NoError(t, err, "creating db")
	require.NoError(t, db.Close(), "closing db")

	fi, err := os.Stat(dbFileLocation)
	require.NoError(t, err, "statting db")

	return fi.ModTime()
}

func Test_rotate(t *testing.T) {
	t.Parallel()

	// Set up test root dir
	tempRootDir := t.TempDir()
	backupDbFileLocation := BackupLauncherDbLocation(tempRootDir)

	// Set up backup saver
	testKnapsack := typesmocks.NewKnapsack(t)
	testKnapsack.On("Slogger").Return(multislogger.NewNopLogger()).Maybe()
	testKnapsack.On("RootDirectory").Return(tempRootDir)
	d := NewDatabaseBackupSaver(testKnapsack)

	for i := 1; i <= numberOfOldBackupsToRetain+1; i += 1 {
		createNonEmptyBboltDb(t, backupDbFileLocation)
		require.NoError(t, d.rotate(), "expected no error on rotation")

		// launcher.db.bak should be renamed to launcher.db.bak.1
		_, err := os.Stat(backupDbFileLocation)
		require.True(t, os.IsNotExist(err), "launcher.db.bak should have been moved to launcher.db.bak.1")

		for j := 1; j <= numberOfOldBackupsToRetain+1; j += 1 {
			currentFilepath := fmt.Sprintf("%s.%d", backupDbFileLocation, j)
			if j > numberOfOldBackupsToRetain {
				// File should never exist
				_, err := os.Stat(currentFilepath)
				require.True(t, os.IsNotExist(err), "%s should not exist -- too many backups retained", currentFilepath)
			} else if j <= i {
				// File should exist
				_, err = os.Stat(currentFilepath)
				require.NoError(t, err, "checking if launcher.db.bak.%d exists", j)
			} else {
				// File should not exist yet
				_, err := os.Stat(currentFilepath)
				require.True(t, os.IsNotExist(err), "%s should not exist yet", currentFilepath)
			}
		}
	}

	// Test rotation when backup db does not exist
	_ = os.Remove(backupDbFileLocation)
	require.NoError(t, d.rotate(), "must be able to rotate even when launcher.db.bak does not exist")
}

func TestInterrupt_Multiple(t *testing.T) {
	t.Parallel()

	testKnapsack := typesmocks.NewKnapsack(t)
	testKnapsack.On("Slogger").Return(multislogger.NewNopLogger())

	d := NewDatabaseBackupSaver(testKnapsack)

	// Start and then interrupt
	go d.Execute()
	d.Interrupt(errors.New("test error"))

	// Confirm we can call Interrupt multiple times without blocking
	interruptComplete := make(chan struct{})
	expectedInterrupts := 3
	for i := 0; i < expectedInterrupts; i += 1 {
		go func() {
			d.Interrupt(nil)
			interruptComplete <- struct{}{}
		}()
	}

	receivedInterrupts := 0
	for {
		if receivedInterrupts >= expectedInterrupts {
			break
		}

		select {
		case <-interruptComplete:
			receivedInterrupts += 1
			continue
		case <-time.After(5 * time.Second):
			t.Errorf("could not call interrupt multiple times and return within 5 seconds -- received %d interrupts before timeout", receivedInterrupts)
			t.FailNow()
		}
	}

	require.Equal(t, expectedInterrupts, receivedInterrupts)
}
