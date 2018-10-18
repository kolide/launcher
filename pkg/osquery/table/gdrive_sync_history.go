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

func GDriveSyncHistoryInfo(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	g := &GDriveSyncHistory{
		client: client,
		logger: logger,
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
	logger log.Logger
}

// GDriveSyncHistoryGenerate will be called whenever the table is queried. It should return
// a full table scan.
func (g *GDriveSyncHistory) generateForPath(ctx context.Context, path string) ([]map[string]string, error) {
	dir, err := ioutil.TempDir("", "kolide_gdrive_sync_history")
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

// GDriveSyncHistoryGenerate will be called whenever the table is queried. It should return
// a full table scan.
func (g *GDriveSyncHistory) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	paths, err := findFileInUserDirs("Library/Application Support/Google/Drive/user_default/snapshot.db")
	if err != nil {
		return nil, errors.Wrap(err, "find gdrive sync history sqlite DBs")
	}

	var results []map[string]string
	for _, path := range paths {
		res, err := g.generateForPath(ctx, path)
		if err != nil {
			level.Info(g.logger).Log(
				"msg", "Generating gdrive history result",
				"path", path,
				"err", err,
			)
			continue
		}
		results = append(results, res...)
	}

	return results, nil
}
