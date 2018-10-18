package table

import (
	"context"
	"database/sql"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"

	"github.com/kolide/kit/fs"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
)

type onePasswordAccountConfig struct {
	client *osquery.ExtensionManagerClient
	logger log.Logger
}

// generate the onepassword account info results given the path to a
// onepassword sqlite DB
func (o *onePasswordAccountConfig) generateForPath(ctx context.Context, path string) ([]map[string]string, error) {
	dir, err := ioutil.TempDir("", "kolide_onepassword_account_config")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir) // clean up

	dst := filepath.Join(dir, "tmpfile")
	if err := fs.CopyFile(path, dst); err != nil {
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

func (o *onePasswordAccountConfig) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	paths, err := findFileInUserDirs("Library/Application Support/1Password 4/Data/B5.sqlite")
	if err != nil {
		return nil, errors.Wrap(err, "find onepassword sqlite DBs")
	}

	var results []map[string]string
	for _, path := range paths {
		res, err := o.generateForPath(ctx, path)
		if err != nil {
			level.Error(o.logger).Log(
				"msg", "Generating onepassword result",
				"path", path,
				"err", err,
			)
			continue
		}
		results = append(results, res...)
	}

	return results, nil
}
