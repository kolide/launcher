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

func TestGDriveSyncConfig(t *testing.T) { //nolint:paralleltest // We need to update package-level vars in this test
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
	tempSqliteFilepath := filepath.Join(tempUserHomeDir, "/Library/Application Support/Google/Drive/user_default/sync_config.db")
	require.NoError(t, os.MkdirAll(filepath.Dir(tempSqliteFilepath), 0755))
	f, err := os.Create(tempSqliteFilepath)
	require.NoError(t, err)
	f.Close()
	db, err := sql.Open("sqlite", tempSqliteFilepath)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS data (entry_key TEXT, data_value TEXT);`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO data (entry_key, data_value) VALUES ("user_email", "testusername@example.com"), ("local_sync_root_path", "test");`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	// Point the table to this new db by modifying package vars
	homeDirLocations[runtime.GOOS] = append(homeDirLocations[runtime.GOOS], tempHomeDir)

	// Create table and verify the name is what we expect
	gdriveTable := GDriveSyncConfig(mockFlags, slogger)
	require.Equal(t, "kolide_gdrive_sync_config", gdriveTable.Name())

	// Confirm we can call the table successfully
	response := gdriveTable.Call(context.TODO(), map[string]string{
		"action":  "generate",
		"context": "{}",
	})
	require.Equal(t, int32(0), response.Status.Code, response.Status.Message) // 0 means success
	testUserFound := false
	for _, row := range response.Response {
		if userEmail, ok := row["user_email"]; ok {
			if userEmail == "testusername@example.com" {
				testUserFound = true
				break
			}
		}
	}
	require.True(t, testUserFound, fmt.Sprintf("response did not include testusername@example.com: %+v", response))
}
