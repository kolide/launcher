package katc

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/kolide/launcher/ee/agent"
	"github.com/osquery/osquery-go/plugin/table"
	_ "modernc.org/sqlite"
)

// sqliteData is the dataFunc for sqlite KATC tables
func sqliteData(ctx context.Context, slogger *slog.Logger, sourcePaths []string, query string, queryContext table.QueryContext) ([]sourceData, error) {
	// Pull out path constraints from the query against the KATC table, to avoid querying more sqlite dbs than we need to.
	pathConstraintsFromQuery := getPathConstraint(queryContext)

	results := make([]sourceData, 0)
	for _, sourcePath := range sourcePaths {
		pathPattern := sourcePatternToGlobbablePattern(sourcePath)
		sqliteDbs, err := filepath.Glob(pathPattern)
		if err != nil {
			return nil, fmt.Errorf("globbing for files with pattern %s: %w", pathPattern, err)
		}

		for _, sqliteDb := range sqliteDbs {
			// Check to make sure `db` adheres to pathConstraintsFromQuery. This is an
			// optimization to avoid work, if osquery sqlite filtering is going to exclude it.
			valid, err := checkPathConstraints(sqliteDb, pathConstraintsFromQuery)
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
	}

	return results, nil
}

// sourcePatternToGlobbablePattern translates the source pattern, which allows for
// using % wildcards for consistency with other osquery tables, into a pattern that can be
// accepted by filepath.Glob.
func sourcePatternToGlobbablePattern(sourcePattern string) string {
	// % matches zero or more characters, corresponds to * in glob syntax
	return strings.Replace(sourcePattern, "%", `*`, -1)
}

// querySqliteDb queries the database at the given path, returning rows of results
func querySqliteDb(ctx context.Context, slogger *slog.Logger, path string, query string) ([]map[string][]byte, error) {
	// If the database is in use, we won't be able to query it. So, copy it to a temporary location first.
	tempDbCopyLocation, err := copySqliteDb(path)
	if err != nil {
		if tempDbCopyLocation != "" {
			_ = os.RemoveAll(tempDbCopyLocation)
		}
		return nil, fmt.Errorf("unable to copy db: %w", err)
	}
	// The copy was successful -- make sure we clean it up after we're done
	defer os.RemoveAll(filepath.Base(tempDbCopyLocation))

	dsn := fmt.Sprintf("file:%s?mode=ro", tempDbCopyLocation)
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

// copySqliteDb makes a temporary directory and copies the given db into it.
func copySqliteDb(path string) (string, error) {
	dbCopyDir, err := agent.MkdirTemp("sqlite-temp")
	if err != nil {
		return "", fmt.Errorf("making temporary directory: %w", err)
	}

	srcFh, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("opening %s: %w", path, err)
	}
	defer srcFh.Close()

	dbCopyDest := filepath.Join(dbCopyDir, filepath.Base(path))
	destFh, err := os.Create(dbCopyDest)
	if err != nil {
		return "", fmt.Errorf("opening %s: %w", dbCopyDest, err)
	}

	if _, err := io.Copy(destFh, srcFh); err != nil {
		_ = destFh.Close()
		return "", fmt.Errorf("copying %s to %s: %w", path, dbCopyDest, err)
	}

	if err := destFh.Close(); err != nil {
		return "", fmt.Errorf("completing write from %s to %s: %w", path, dbCopyDest, err)
	}

	return dbCopyDest, nil
}
