package katc

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/osquery/osquery-go/plugin/table"
	_ "modernc.org/sqlite"
)

// sqliteData is the dataFunc for sqlite KATC tables
func sqliteData(ctx context.Context, slogger *slog.Logger, pathPattern string, query string, sourceConstraints *table.ConstraintList) ([]sourceData, error) {
	sqliteDbs, err := filepath.Glob(pathPattern)
	if err != nil {
		return nil, fmt.Errorf("globbing for files with pattern %s: %w", pathPattern, err)
	}

	results := make([]sourceData, 0)
	for _, sqliteDb := range sqliteDbs {
		// Check to make sure `sqliteDb` adheres to sourceConstraints
		valid, err := checkSourcePathConstraints(sqliteDb, sourceConstraints)
		if err != nil {
			return nil, fmt.Errorf("checking source path constraints: %w", err)
		}
		if !valid {
			continue
		}

		rowsFromDb, err := querySqliteDb(ctx, slogger, sqliteDb, query)
		if err != nil {
			return nil, fmt.Errorf("querying %s: %w", sqliteDb, err)
		}
		results = append(results, sourceData{
			path: sqliteDb,
			rows: rowsFromDb,
		})
	}

	return results, nil
}

// querySqliteDb queries the database at the given path, returning rows of results
func querySqliteDb(ctx context.Context, slogger *slog.Logger, path string, query string) ([]map[string][]byte, error) {
	dsn := fmt.Sprintf("file://%s?mode=ro", path)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite db: %w", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			slogger.Log(ctx, slog.LevelWarn,
				"closing sqlite db after query",
				"err", err,
			)
		}
	}()

	rows, err := conn.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("running query: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slogger.Log(ctx, slog.LevelWarn,
				"closing rows after scanning results",
				"err", err,
			)
		}
	}()

	results := make([]map[string][]byte, 0)

	// Fetch columns so we know how many values per row we will scan
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("getting columns from query result: %w", err)
	}

	// Prepare scan destination
	rawResult := make([][]byte, len(columns))
	scanDest := make([]any, len(columns))
	for i := 0; i < len(columns); i += 1 {
		scanDest[i] = &rawResult[i]
	}

	// Scan all rows
	for rows.Next() {
		if err := rows.Scan(scanDest...); err != nil {
			return nil, fmt.Errorf("scanning query results: %w", err)
		}

		row := make(map[string][]byte)
		for i := 0; i < len(columns); i += 1 {
			row[columns[i]] = rawResult[i]
		}

		results = append(results, row)
	}

	return results, nil
}
