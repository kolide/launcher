package table

import (
	"context"
	"database/sql"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/fs"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

func GDriveSyncConfig(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {
	g := &gdrive{
		client: client,
		logger: logger,
	}

	columns := []table.ColumnDefinition{
		table.TextColumn("user_email"),
		table.TextColumn("local_sync_root_path"),
	}
	return table.NewPlugin("kolide_gdrive_sync_config", columns, g.generate)
}

type gdrive struct {
	client *osquery.ExtensionManagerClient
	logger log.Logger
}

func (g *gdrive) generateForPath(ctx context.Context, path string) ([]map[string]string, error) {
	dir, err := ioutil.TempDir("", "kolide_gdrive_sync_config")
	if err != nil {
		return nil, errors.Wrap(err, "creating kolide_gdrive_sync_config tmp dir")
	}
	defer os.RemoveAll(dir) // clean up

	dst := filepath.Join(dir, "tmpfile")
	if err := fs.CopyFile(path, dst); err != nil {
		return nil, errors.Wrap(err, "copying sqlite db to tmp dir")
	}

	db, err := sql.Open("sqlite3", dst)
	if err != nil {
		return nil, errors.Wrap(err, "connecting to sqlite db")
	}
	defer db.Close()

	db.Exec("PRAGMA journal_mode=WAL;")

	rows, err := db.Query(
		`SELECT entry_key, data_value
		FROM data
		WHERE entry_key = 'user_email' OR entry_key='local_sync_root_path'
			AND data_value IS NOT NULL`)
	if err != nil {
		return nil, errors.Wrap(err, "query rows from gdrive sync config db")
	}
	defer rows.Close()

	var email string
	var localsyncpath string
	for rows.Next() {
		var (
			entryKey  string
			dataValue string
		)
		if err := rows.Scan(&entryKey, &dataValue); err != nil {
			return nil, errors.Wrap(err, "scanning gdrive sync config db row")
		}

		switch entryKey {
		case "user_email":
			email = dataValue
		case "local_sync_root_path":
			localsyncpath = dataValue
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

func (g *gdrive) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	files, err := findFileInUserDirs("/Library/Application Support/Google/Drive/user_default/sync_config.db", g.logger)
	if err != nil {
		return nil, errors.Wrap(err, "find gdrive sync config sqlite DBs")
	}

	var results []map[string]string
	for _, file := range files {
		res, err := g.generateForPath(ctx, file.path)
		if err != nil {
			level.Info(g.logger).Log(
				"msg", "Generating gdrive sync result",
				"path", file.path,
				"err", err,
			)
			continue
		}
		results = append(results, res...)
	}

	return results, nil
}
