package table

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestGDriveSyncHistoryInfo(t *testing.T) { //nolint:paralleltest // We need to update package-level vars in this test
	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(t)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	// Set up a sqlite database for querying
	testUsername := "testusername"
	tempHomeDir := t.TempDir()
	tempUserHomeDir := filepath.Join(tempHomeDir, testUsername)
	require.NoError(t, os.Mkdir(tempUserHomeDir, 0755))
	tempSqliteFilepath := filepath.Join(tempUserHomeDir, "Library/Application Support/Google/Drive/user_default/snapshot.db")
	require.NoError(t, os.MkdirAll(filepath.Dir(tempSqliteFilepath), 0755))
	f, err := os.Create(tempSqliteFilepath)
	require.NoError(t, err)
	f.Close()
	db, err := sql.Open("sqlite", tempSqliteFilepath)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS local_entry (inode TEXT, filename TEXT, modified TEXT, size TEXT, checksum TEXT);`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO local_entry (inode, filename, modified, size, checksum) VALUES ("", "testfile", "", "", "");`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS cloud_entry (inode TEXT, filename TEXT, modified TEXT, size TEXT, checksum TEXT);`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO cloud_entry (inode, filename, modified, size, checksum) VALUES ("", "testfile", "", "", "");`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	// Point the table to this new db by modifying package vars
	homeDirLocations[runtime.GOOS] = append(homeDirLocations[runtime.GOOS], tempHomeDir)

	// Create table and verify the name is what we expect
	gdriveHistoryTable := GDriveSyncHistoryInfo(mockFlags, slogger)
	require.Equal(t, "kolide_gdrive_sync_history", gdriveHistoryTable.Name())

	// Confirm we can call the table successfully
	response := gdriveHistoryTable.Call(context.TODO(), map[string]string{
		"action":  "generate",
		"context": "{}",
	})
	require.Equal(t, int32(0), response.Status.Code, response.Status.Message) // 0 means success
	testFileFound := false
	for _, row := range response.Response {
		if fileName, ok := row["filename"]; ok {
			if fileName == "testfile" {
				testFileFound = true
				break
			}
		}
	}
	require.True(t, testFileFound, fmt.Sprintf("response did not include testfile: %+v", response))
}
