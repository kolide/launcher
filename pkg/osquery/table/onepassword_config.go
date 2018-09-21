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

func OnePasswordAccountConfigInfo(client *osquery.ExtensionManagerClient) *table.Plugin {
	o := &OnePasswordAccountConfig{
		client: client,
	}
	columns := []table.ColumnDefinition{
		table.TextColumn("team_name"),
		table.TextColumn("user_email"),
		table.TextColumn("user_first_name"),
		table.TextColumn("user_last_name"),
	}
	return table.NewPlugin("kolide_onepassword_account_config", columns, o.generate)
}

type OnePasswordAccountConfig struct {
	client *osquery.ExtensionManagerClient
}

// OnePasswordAccountConfigGenerate will be called whenever the table is queried. It should return
// a full table scan.
func (o *OnePasswordAccountConfig) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	paths, err := queryDbPath(o.client)
	if err != nil {
		return nil, err
	}

	dir, err := ioutil.TempDir("", "kolide_onepassword_account_config")
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

	rows, err := db.Query("SELECT team_name, user_email, user_first_name, user_last_name FROM accounts")
	if err != nil {
		return nil, errors.Wrap(err, "query rows from onepassword account configuration db")
	}
	defer rows.Close()

	var results []map[string]string

	// loop through all the sqlite rows and add them as osquery rows in the results map
	for rows.Next() { // we initialize these variables for every row, that way we don't have data from the previous iteration
		var team_name string
		var user_email string
		var user_first_name string
		var user_last_name string
		if err := rows.Scan(&team_name, &user_email, &user_first_name, &user_last_name); err != nil {
			return nil, errors.Wrap(err, "scanning onepassword account configuration db row")
		}

		results = append(results, map[string]string{
			"team_name":       team_name,
			"user_email":      user_email,
			"user_first_name": user_first_name,
			"user_last_name":  user_last_name,
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
	path := filepath.Join("/Users", username, "/Library/Application Support/1Password 4/Data/B5.sqlite")
	return path, nil
}
