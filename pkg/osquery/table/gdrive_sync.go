package table

import (
	"context"
	"database/sql"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/kolide/kit/fs"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"

	_ "github.com/mattn/go-sqlite3"
)

func GDriveSyncConfig(client *osquery.ExtensionManagerClient) *table.Plugin {
	g := &gdrive{
		client: client,
	}

	columns := []table.ColumnDefinition{
		table.TextColumn("user_email"),
		table.TextColumn("local_sync_root_path"),
	}
	return table.NewPlugin("kolide_gdrive_sync_config", columns, g.generate)
}

type gdrive struct {
	client *osquery.ExtensionManagerClient
}

// generate will be called whenever the table is queried. It should return a
// full table scan.
func (g *gdrive) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	user, err := getPrimaryUser(g.client)
	if err != nil {
		return nil, errors.Wrap(err, "get primary user for gdrive sync info")
	}
	paths := filepath.Join("/Users", user, "/Library/Application Support/Google/Drive/user_default/sync_config.db")

	dir, err := ioutil.TempDir("", "kolide_gdrive_sync_config")
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

	rows, err := db.Query("SELECT entry_key, data_key, data_value FROM data")
	if err != nil {
		return nil, errors.Wrap(err, "query rows from gdrive sync config db")
	}
	defer rows.Close()

	var email string
	var localsyncpath string
	for rows.Next() {
		var (
			entry_key  string
			data_key   string
			data_value string
		)
		if err := rows.Scan(&entry_key, &data_key, &data_value); err != nil {
			return nil, errors.Wrap(err, "scanning gdrive sync config db row")
		}

		switch entry_key {
		case "user_email":
			email = data_value
		case "local_sync_root_path":
			localsyncpath = data_value
		default:
			continue
		}
	}
	return []map[string]string{
		{
			"user_email":           email,
			"local_sync_root_path": localsyncpath,
		},
	}, nil
}
