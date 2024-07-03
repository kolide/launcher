package katc

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/kolide/launcher/ee/indexeddb"
	"github.com/osquery/osquery-go/plugin/table"
)

// indexeddbLeveldbData retrieves data from the LevelDB-backed IndexedDB instances
// found at the filepath in `sourcePattern`. It retrieves all rows from the database
// and object store specified in `query`, which it expects to be in the format
// `<db name>.<object store name>`.
func indexeddbLeveldbData(ctx context.Context, slogger *slog.Logger, sourcePattern string, query string, sourceConstraints *table.ConstraintList) ([]sourceData, error) {
	pathPattern := sourcePatternToGlobbablePattern(sourcePattern)
	leveldbs, err := filepath.Glob(pathPattern)
	if err != nil {
		return nil, fmt.Errorf("globbing for leveldb files: %w", err)
	}

	// Extract database and table from query
	dbName, objectStoreName, err := extractQueryTargets(query)
	if err != nil {
		return nil, fmt.Errorf("getting db and object store names: %w", err)
	}

	// Query databases
	results := make([]sourceData, 0)
	for _, db := range leveldbs {
		// Check to make sure `db` adheres to sourceConstraints
		valid, err := checkSourceConstraints(db, sourceConstraints)
		if err != nil {
			return nil, fmt.Errorf("checking source path constraints: %w", err)
		}
		if !valid {
			continue
		}

		rowsFromDb, err := indexeddb.QueryIndexeddbObjectStore(db, dbName, objectStoreName)
		if err != nil {
			return nil, fmt.Errorf("querying %s: %w", db, err)
		}
		results = append(results, sourceData{
			path: db,
			rows: rowsFromDb,
		})
	}

	return results, nil
}

// extractQueryTargets retrieves the targets of the query (the database name and the object store name)
// from the query. IndexedDB is a NoSQL database, so we expect to retrieve all rows from the given
// object store within the given database name.
func extractQueryTargets(query string) (string, string, error) {
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
