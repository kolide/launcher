package table

import (
	"context"
	"database/sql"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"

	_ "github.com/mattn/go-sqlite3"
)

func ChromeLoginKeychainInfo(client *osquery.ExtensionManagerClient) *table.Plugin {
	//figure it out
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
	// take a client instead of db
	client *osquery.ExtensionManagerClient
}

// this function is not necessary, use columns := to just return the column definition in the top function
/* func (c *ChromeLoginKeychain) ChromeLoginKeychainColumns() []table.ColumnDefinition {
	return []table.ColumnDefinition{
		table.TextColumn("origin_url"),
		table.TextColumn("action_url"),
		table.TextColumn("username_value"),
	}
} */

// ChromeLoginKeychainGenerate will be called whenever the table is queried. It should return
// a full table scan.
func (c *ChromeLoginKeychain) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	paths, err := queryDbPath(c.client)
	if err != nil {
		return nil, err
	}
	// open and close db here, same as other table
	db, err := sql.Open("sqlite3", paths)
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
		rows.Scan(&origin_url, &action_url, &username_value) // handle this error
		if err := rows.Scan(&origin_url, &action_url, &username_value); err != nil {
			return nil, errors.Wrap(err, "scanning chrome login keychain db row")
		}

		results = append(results, map[string]string{
			"origin_url":     origin_url,
			"action_url":     action_url,
			"username_value": username_value,
		})
	}
	return results, nil //no error
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
