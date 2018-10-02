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
)

type onePasswordAccountConfig struct {
	client *osquery.ExtensionManagerClient
}

// OnePasswordAccountConfigGenerate will be called whenever the table is queried. It should return
// a full table scan.
func (o *onePasswordAccountConfig) generate(ctx context.Context, queryContext table.QueryContext, results []map[string]string) ([]map[string]string, error) {
	paths, err := queryDbPath(o.client)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(paths); os.IsNotExist(err) {
		return results, nil // only populate results if path exists
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

	rows, err := db.Query("SELECT user_email FROM accounts")
	if err != nil {
		return nil, errors.Wrap(err, "query rows from onepassword account configuration db")
	}
	defer rows.Close()

	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, errors.Wrap(err, "scanning onepassword account configuration db row")
		}
		if email == "" {
			continue
		}
		results = addEmailToResults(email, results)
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
