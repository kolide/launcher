package table

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/kolide/kit/fsutil"
	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/table"
	_ "modernc.org/sqlite"
)

func GDriveSyncConfig(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	g := &gdrive{
		slogger: slogger.With("table", "kolide_gdrive_sync_config"),
	}

	columns := []table.ColumnDefinition{
		table.TextColumn("user_email"),
		table.TextColumn("local_sync_root_path"),
	}
	return tablewrapper.New(flags, slogger, "kolide_gdrive_sync_config", columns, g.generate)
}

type gdrive struct {
	slogger *slog.Logger
}

func (g *gdrive) generateForPath(ctx context.Context, path string) ([]map[string]string, error) {
	_, span := traces.StartSpan(ctx, "path", path)
	defer span.End()

	dir, err := agent.MkdirTemp("kolide_gdrive_sync_config")
	if err != nil {
		return nil, fmt.Errorf("creating kolide_gdrive_sync_config tmp dir: %w", err)
	}
	defer os.RemoveAll(dir) // clean up

	dst := filepath.Join(dir, "tmpfile")
	if err := fsutil.CopyFile(path, dst); err != nil {
		return nil, fmt.Errorf("copying sqlite db to tmp dir: %w", err)
	}

	db, err := sql.Open("sqlite", dst)
	if err != nil {
		return nil, fmt.Errorf("connecting to sqlite db: %w", err)
	}
	defer db.Close()

	db.Exec("PRAGMA journal_mode=WAL;")

	rows, err := db.Query(
		`SELECT entry_key, data_value
		FROM data
		WHERE entry_key = 'user_email' OR entry_key='local_sync_root_path'
			AND data_value IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("query rows from gdrive sync config db: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			g.slogger.Log(ctx, slog.LevelWarn,
				"closing rows after scanning results",
				"err", err,
			)
		}
		if err := rows.Err(); err != nil {
			g.slogger.Log(ctx, slog.LevelWarn,
				"encountered iteration error",
				"err", err,
			)
		}
	}()

	var email string
	var localsyncpath string
	for rows.Next() {
		var (
			entryKey  string
			dataValue string
		)
		if err := rows.Scan(&entryKey, &dataValue); err != nil {
			return nil, fmt.Errorf("scanning gdrive sync config db row: %w", err)
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
	ctx, span := traces.StartSpan(ctx, "table_name", "kolide_gdrive_sync_config")
	defer span.End()

	files, err := findFileInUserDirs("/Library/Application Support/Google/Drive/user_default/sync_config.db", g.slogger)
	if err != nil {
		return nil, fmt.Errorf("find gdrive sync config sqlite DBs: %w", err)
	}

	var results []map[string]string
	for _, file := range files {
		res, err := g.generateForPath(ctx, file.path)
		if err != nil {
			g.slogger.Log(ctx, slog.LevelInfo,
				"generating gdrive sync result",
				"path", file.path,
				"err", err,
			)
			continue
		}
		results = append(results, res...)
	}

	return results, nil
}
