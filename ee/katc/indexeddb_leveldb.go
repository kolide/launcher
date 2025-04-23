package katc

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/kolide/launcher/ee/indexeddb"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/table"
)

// indexeddbLeveldbData retrieves data from the LevelDB-backed IndexedDB instances
// found at the filepath in `sourcePattern`. It retrieves all rows from the database
// and object store specified in `query`, which it expects to be in the format
// `<db name>.<object store name>`.
func indexeddbLeveldbData(ctx context.Context, slogger *slog.Logger, sourcePaths []string, query string, queryContext table.QueryContext) ([]sourceData, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	// Pull out path constraints from the query against the KATC table, to avoid querying more leveldb files than we need to.
	pathConstraintsFromQuery := getPathConstraint(queryContext)

	results := make([]sourceData, 0)
	for _, sourcePath := range sourcePaths {
		pathPattern := sourcePatternToGlobbablePattern(sourcePath)
		leveldbs, err := filepath.Glob(pathPattern)
		if err != nil {
			return nil, fmt.Errorf("globbing for leveldb files: %w", err)
		}

		// Extract database and table from query
		dbName, objectStoreName, err := extractIndexeddbQueryTargets(query)
		if err != nil {
			return nil, fmt.Errorf("getting db and object store names: %w", err)
		}

		// Query databases
		for _, db := range leveldbs {
			// Check to make sure `db` adheres to pathConstraintsFromQuery. This is an
			// optimization to avoid work, if osquery sqlite filtering is going to exclude it.
			valid, err := checkPathConstraints(db, pathConstraintsFromQuery)
			if err != nil {
				return nil, fmt.Errorf("checking source path constraints: %w", err)
			}
			if !valid {
				continue
			}

			rowsFromDb, err := indexeddb.QueryIndexeddbObjectStore(ctx, db, dbName, objectStoreName)
			if err != nil {
				return nil, fmt.Errorf("querying %s: %w", db, err)
			}
			results = append(results, sourceData{
				path: db,
				rows: rowsFromDb,
			})
		}
	}

	return results, nil
}

// extractIndexeddbQueryTargets retrieves the targets of the query (the database name and the object store name)
// from the query. IndexedDB is a NoSQL database, so we expect to retrieve all rows from the given
// object store within the given database name.
func extractIndexeddbQueryTargets(query string) (string, string, error) {
	parts := strings.Split(query, ".")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("unable to extract query targets from query: expected `<db name>.<obj store name>`, got `%s`", query)
	}
	if len(parts[0]) == 0 {
		return "", "", fmt.Errorf("missing db name in query `%s`", query)
	}
	if len(parts[1]) == 0 {
		return "", "", fmt.Errorf("missing object store name in query `%s`", query)
	}
	return parts[0], parts[1], nil
}
