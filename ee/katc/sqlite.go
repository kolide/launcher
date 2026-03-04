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

	"github.com/kolide/launcher/v2/ee/agent"
	"github.com/kolide/launcher/v2/ee/observability"
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
	return strings.ReplaceAll(sourcePattern, "%", `*`)
}

// querySqliteDb queries the database at the given path, returning rows of results
func querySqliteDb(ctx context.Context, slogger *slog.Logger, path string, query string) ([]map[string][]byte, error) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	// If the database is in use, we won't be able to query it. So, copy it to a temporary location first.
	tempDbCopyLocation, err := copySqliteDb(path)
	if err != nil {
		if tempDbCopyLocation != "" {
			_ = os.RemoveAll(filepath.Dir(tempDbCopyLocation))
		}
		return nil, fmt.Errorf("unable to copy db: %w", err)
	}
	// The copy was successful -- make sure we clean it up after we're done
	defer os.RemoveAll(filepath.Dir(tempDbCopyLocation))

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

// copySqliteDb makes a temporary directory and copies the given db into it,
// along with any WAL and SHM auxiliary files.
func copySqliteDb(path string) (string, error) {
	dbCopyDir, err := agent.MkdirTemp("sqlite-temp")
	if err != nil {
		return "", fmt.Errorf("making temporary directory: %w", err)
	}

	dbCopyDest, err := copyFile(path, dbCopyDir)
	if err != nil {
		return "", fmt.Errorf("copying %s: %w", path, err)
	}

	for _, suffix := range []string{"-wal", "-shm"} {
		auxPath := path + suffix
		if _, err := os.Stat(auxPath); err == nil {
			if _, err := copyFile(auxPath, dbCopyDir); err != nil {
				return dbCopyDest, fmt.Errorf("copying %s: %w", suffix, err)
			}
		}
	}

	return dbCopyDest, nil
}

// copyFile copies a single file into destDir, preserving the base filename.
func copyFile(srcPath string, destDir string) (string, error) {
	srcFh, err := os.Open(srcPath)
	if err != nil {
		return "", fmt.Errorf("opening %s: %w", srcPath, err)
	}
	defer srcFh.Close()

	destPath := filepath.Join(destDir, filepath.Base(srcPath))
	destFh, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("creating %s: %w", destPath, err)
	}

	if _, err := io.Copy(destFh, srcFh); err != nil {
		_ = destFh.Close()
		return "", fmt.Errorf("copying %s to %s: %w", srcPath, destPath, err)
	}

	if err := destFh.Close(); err != nil {
		return "", fmt.Errorf("writing %s to %s: %w", srcPath, destPath, err)
	}

	return destPath, nil
}
