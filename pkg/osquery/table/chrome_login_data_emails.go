package table

import (
	"context"
	"database/sql"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"

	"github.com/kolide/kit/fs"
	"github.com/osquery/osquery-go"
	"github.com/osquery/osquery-go/plugin/table"
)

var profileDirs = map[string][]string{
	"windows": []string{"Appdata/Local/Google/Chrome/User Data"},
	"darwin":  []string{"Library/Application Support/Google/Chrome"},
}
var profileDirsDefault = []string{".config/google-chrome", ".config/chromium", "snap/chromium/current/.config/chromium"}

func ChromeLoginDataEmails(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	c := &ChromeLoginDataEmailsTable{
		client: client,
		logger: logger,
	}
	columns := []table.ColumnDefinition{
		table.TextColumn("username"),
		table.TextColumn("email"),
		table.BigIntColumn("count"),
	}
	return table.NewPlugin("kolide_chrome_login_data_emails", columns, c.generate)
}

type ChromeLoginDataEmailsTable struct {
	client *osquery.ExtensionManagerClient
	logger log.Logger
}

func (c *ChromeLoginDataEmailsTable) generateForPath(ctx context.Context, file userFileInfo) ([]map[string]string, error) {
	dir, err := ioutil.TempDir("", "kolide_chrome_login_data_emails")
	if err != nil {
		return nil, errors.Wrap(err, "creating kolide_chrome_login_data_emails tmp dir")
	}
	defer os.RemoveAll(dir) // clean up

	dst := filepath.Join(dir, "tmpfile")
	if err := fs.CopyFile(file.path, dst); err != nil {
		return nil, errors.Wrap(err, "copying sqlite file to tmp dir")
	}

	db, err := sql.Open("sqlite3", dst)
	if err != nil {
		return nil, errors.Wrap(err, "connecting to sqlite db")
	}
	defer db.Close()

	rows, err := db.Query("SELECT username_value, count(*) AS count FROM logins GROUP BY lower(username_value)")
	if err != nil {
		return nil, errors.Wrap(err, "query rows from chrome login keychain db")
	}
	defer rows.Close()

	var results []map[string]string

	// loop through all the sqlite rows and add them as osquery rows in the results map
	for rows.Next() { // we initialize these variables for every row, that way we don't have data from the previous iteration
		var username_value string
		var username_count string
		if err := rows.Scan(&username_value, &username_count); err != nil {
			return nil, errors.Wrap(err, "scanning chrome login keychain db row")
		}
		// append anything that could be an email
		if !strings.Contains(username_value, "@") {
			continue
		}
		results = append(results, map[string]string{
			"username": file.user,
			"email":    username_value,
			"count":    username_count,
		})
	}
	return results, nil
}

func (c *ChromeLoginDataEmailsTable) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string
	osProfileDirs, ok := profileDirs[runtime.GOOS]
	if !ok {
		osProfileDirs = profileDirsDefault
	}

	for _, profileDir := range osProfileDirs {
		files, err := findFileInUserDirs(filepath.Join(profileDir, "*/Login Data"), c.logger)
		if err != nil {
			level.Info(c.logger).Log(
				"msg", "Find chrome login data sqlite DBs",
				"path", profileDir,
				"err", err,
			)
			continue
		}

		for _, file := range files {
			res, err := c.generateForPath(ctx, file)
			if err != nil {
				level.Info(c.logger).Log(
					"msg", "Generating chrome keychain result",
					"path", file.path,
					"err", err,
				)
				continue
			}
			results = append(results, res...)
		}
	}

	return results, nil
}
