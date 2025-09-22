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

func TestChromeLoginDataEmails(t *testing.T) { //nolint:paralleltest // We need to update package-level vars in this test
	// Set up table dependencies
	mockFlags := typesmocks.NewFlags(t)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()
	slogger := multislogger.NewNopLogger()

	// Set up a sqlite database for querying
	// It must live in <home>/<username>/<app>/*/Login Data
	testUsername := "testusername"
	tempHomeDir := t.TempDir()
	tempUserHomeDir := filepath.Join(tempHomeDir, testUsername)
	require.NoError(t, os.Mkdir(tempUserHomeDir, 0755))
	appDir := "chrome"
	require.NoError(t, os.Mkdir(filepath.Join(tempUserHomeDir, appDir), 0755))
	profileDir := filepath.Join(tempUserHomeDir, appDir, "testprofile")
	require.NoError(t, os.Mkdir(profileDir, 0755))
	tempSqliteFilepath := filepath.Join(profileDir, "Login Data")
	f, err := os.Create(tempSqliteFilepath)
	require.NoError(t, err)
	f.Close()
	db, err := sql.Open("sqlite", tempSqliteFilepath)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS logins (username_value TEXT);`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO logins (username_value) VALUES ("testusername@example.com");`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	// Point the table to this new db by modifying package vars
	homeDirLocations[runtime.GOOS] = append(homeDirLocations[runtime.GOOS], tempHomeDir)
	profileDirs[runtime.GOOS] = append(profileDirs[runtime.GOOS], appDir)

	// Create table and verify the name is what we expect
	chromeLoginDataEmailsTable := ChromeLoginDataEmails(mockFlags, slogger)
	require.Equal(t, "kolide_chrome_login_data_emails", chromeLoginDataEmailsTable.Name())

	// Confirm we can call the table successfully
	response := chromeLoginDataEmailsTable.Call(context.TODO(), map[string]string{
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
