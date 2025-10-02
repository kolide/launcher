package table

import (
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

func TestOnePasswordAccounts(t *testing.T) { //nolint:paralleltest // We need to update package-level vars in this test
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
	tempSqliteFilepath := filepath.Join(tempUserHomeDir, "test1passworddb.sqlite")
	f, err := os.Create(tempSqliteFilepath)
	require.NoError(t, err)
	f.Close()
	db, err := sql.Open("sqlite", tempSqliteFilepath)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS accounts (user_email TEXT, team_name TEXT, server TEXT, user_first_name TEXT, user_last_name TEXT, account_type TEXT);`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO accounts (user_email, team_name, server, user_first_name, user_last_name, account_type) VALUES ("testusername@example.com", "Test Team", "myteam.example.com", "Jane", "Test", "");`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	// Point the table to this new db by modifying package vars
	homeDirLocations[runtime.GOOS] = append(homeDirLocations[runtime.GOOS], tempHomeDir)
	onepasswordDataFiles[runtime.GOOS] = append(onepasswordDataFiles[runtime.GOOS], "test1passworddb.sqlite")

	// Create table and verify the name is what we expect
	accountsTable := OnePasswordAccounts(mockFlags, slogger)
	require.Equal(t, "kolide_onepassword_accounts", accountsTable.Name())

	// Confirm we can call the table successfully
	response := accountsTable.Call(t.Context(), map[string]string{
		"action":  "generate",
		"context": "{}",
	})
	require.Equal(t, int32(0), response.Status.Code, response.Status.Message) // 0 means success
	testUserFound := false
	for _, row := range response.Response {
		if username, ok := row["username"]; ok {
			if username == testUsername {
				testUserFound = true
				break
			}
		}
	}
	require.True(t, testUserFound, fmt.Sprintf("response did not include testusername: %+v", response))
}
