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
func (o *onePasswordAccountConfig) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	user, err := getPrimaryUser(o.client)
	if err != nil {
		return nil, errors.Wrap(err, "get primary user for onepassword config")
	}
	paths := filepath.Join("/Users", user, "/Library/Application Support/1Password 4/Data/B5.sqlite")

	if _, err := os.Stat(paths); os.IsNotExist(err) {
		return nil, nil // only populate results if path exists
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

	var results []map[string]string
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
