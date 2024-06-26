package katc

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// sqliteData is the dataFunc for sqlite KATC tables
func sqliteData(ctx context.Context, pathPattern string, query string, columns []string, slogger *slog.Logger) ([]map[string][]byte, error) {
	sqliteDbs, err := filepath.Glob(pathPattern)
	if err != nil {
		return nil, fmt.Errorf("globbing for files with pattern %s: %w", pathPattern, err)
	}

	results := make([]map[string][]byte, 0)
	for _, sqliteDb := range sqliteDbs {
		resultsFromDb, err := querySqliteDb(ctx, sqliteDb, query, columns, slogger)
		if err != nil {
			return nil, fmt.Errorf("querying %s: %w", sqliteDb, err)
		}
		results = append(results, resultsFromDb...)
	}

	return results, nil
}

// querySqliteDb queries the database at the given path, returning rows of results
func querySqliteDb(ctx context.Context, path string, query string, columns []string, slogger *slog.Logger) ([]map[string][]byte, error) {
	conn, err := sql.Open("sqlite", path)
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

	// Prepare destination for scan
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
