package katc

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/kolide/launcher/ee/observability"
	"github.com/osquery/osquery-go/plugin/table"
	_ "modernc.org/sqlite"
)

// sqliteData is the dataFunc for sqlite KATC tables
func sqliteData(ctx context.Context, slogger *slog.Logger, sourcePaths []string, query string, queryContext table.QueryContext) ([]sourceData, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

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
				slogger.Log(ctx, slog.LevelWarn,
					"could not query sqlite database at path",
					"sqlite_db_path", sqliteDb,
					"err", err,
				)
				continue
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
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	dsn := fmt.Sprintf("file:%s?mode=ro&immutable=1", path)
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
		if err := rows.Err(); err != nil {
			slogger.Log(ctx, slog.LevelWarn,
				"encountered iteration error",
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
