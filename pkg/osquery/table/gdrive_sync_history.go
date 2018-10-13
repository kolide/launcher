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

func GDriveSyncHistoryInfo(client *osquery.ExtensionManagerClient) *table.Plugin {
	g := &GDriveSyncHistory{
		client: client,
	}
	columns := []table.ColumnDefinition{
		table.TextColumn("inode"),
		table.TextColumn("filename"),
		table.TextColumn("mtime"),
		table.TextColumn("size"),
	}
	return table.NewPlugin("kolide_gdrive_sync_history", columns, g.generate)
}

type GDriveSyncHistory struct {
	client *osquery.ExtensionManagerClient
}

// GDriveSyncHistoryGenerate will be called whenever the table is queried. It should return
// a full table scan.
func (g *GDriveSyncHistory) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	user, err := getPrimaryUser(g.client)
	if err != nil {
		return nil, errors.Wrap(err, "get primary user for gdrive sync history")
	}
	paths := filepath.Join("/Users", user, "/Library/Application Support/Google/Drive/user_default/snapshot.db")

	dir, err := ioutil.TempDir("", "kolide_gdrive_sync_history")
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

	rows, err := db.Query("select distinct le.inode, le.filename, le.modified AS mtime, le.size from local_entry le, cloud_entry ce using (checksum) order by le.modified desc;")
	if err != nil {
		return nil, errors.Wrap(err, "query rows from gdrive sync history db")
	}
	defer rows.Close()

	var results []map[string]string

	// loop through all the sqlite rows and add them as osquery rows in the results map
	for rows.Next() { // we initialize these variables for every row, that way we don't have data from the previous iteration
		var inode string
		var filename string
		var mtime string
		var size string
		if err := rows.Scan(&inode, &filename, &mtime, &size); err != nil {
			return nil, errors.Wrap(err, "scanning gdrive sync history db row")
		}

		results = append(results, map[string]string{
			"inode":    inode,
			"filename": filename,
			"mtime":    mtime,
			"size":     size,
		})
	}
	return results, nil
}
