package table

import (
	"context"
	"database/sql"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/kolide/kit/fs"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"

	_ "github.com/mattn/go-sqlite3"
)

func ChromeLoginKeychainInfo(client *osquery.ExtensionManagerClient) *table.Plugin {
	c := &ChromeLoginKeychain{
		client: client,
	}
	columns := []table.ColumnDefinition{
		table.TextColumn("origin_url"),
		table.TextColumn("action_url"),
		table.TextColumn("username_value"),
	}
	return table.NewPlugin("kolide_chrome_login_keychain", columns, c.generate)
}

type ChromeLoginKeychain struct {
	client *osquery.ExtensionManagerClient
}

// ChromeLoginKeychainGenerate will be called whenever the table is queried. It should return
// a full table scan.
func (c *ChromeLoginKeychain) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	paths, err := queryDbPath(c.client)
	if err != nil {
		return nil, err
	}

	dir, err := ioutil.TempDir("", "kolide_chrome_login_keychain")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir) // clean up

	dst := filepath.Join(dir, "tmpfile")
	if err := fs.CopyFile(paths, dst); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", dst)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	db.Exec("PRAGMA journal_mode=WAL;")

	rows, err := db.Query("SELECT origin_url, action_url, username_value FROM logins")
	if err != nil {
		return nil, errors.Wrap(err, "query rows from chrome login keychain db")
	}
	defer rows.Close()

	var results []map[string]string

	// loop through all the sqlite rows and add them as osquery rows in the results map
	for rows.Next() { // we initialize these variables for every row, that way we don't have data from the previous iteration
		var origin_url string
		var action_url string
		var username_value string
		if err := rows.Scan(&origin_url, &action_url, &username_value); err != nil {
			return nil, errors.Wrap(err, "scanning chrome login keychain db row")
		}

		results = append(results, map[string]string{
			"origin_url":     origin_url,
			"action_url":     action_url,
			"username_value": username_value,
		})
	}
	return results, nil
}

func queryDbPath(client *osquery.ExtensionManagerClient) (string, error) {
	query := `select username from last where username not in ('', 'root') group by username order by count(username) desc limit 1`
	row, err := client.QueryRow(query)
	if err != nil {
		return "", errors.Wrap(err, "querying for primaryUser version")
	}
	var username string
	if val, ok := row["username"]; ok {
		username = val
	}
	path := filepath.Join("/Users", username, "/Library/Application Support/Google/Chrome/Default/Login Data")
	return path, nil
}
