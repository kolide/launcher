package katc

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/kolide/launcher/ee/indexeddb"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/table"
)

// leveldbData is the dataFunc for plain LevelDB databases without additional encoding
// or nesting. IndexedDB databases leveraging LevelDB should use `indexeddbLeveldbData`
// instead. Since leveldbData assumes a key-value store without nesting, the `query` is
// ignored here. All key-value pairs in the database are returned.
func leveldbData(ctx context.Context, slogger *slog.Logger, sourcePaths []string, _ string, queryContext table.QueryContext) ([]sourceData, error) {
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

		// Query databases
		for _, dbPath := range leveldbs {
			// Check to make sure `db` adheres to pathConstraintsFromQuery. This is an
			// optimization to avoid work, if osquery sqlite filtering is going to exclude it.
			valid, err := checkPathConstraints(dbPath, pathConstraintsFromQuery)
			if err != nil {
				return nil, fmt.Errorf("checking source path constraints: %w", err)
			}
			if !valid {
				continue
			}

			rowsFromDb, err := queryLeveldb(ctx, dbPath)
			if err != nil {
				return nil, fmt.Errorf("querying leveldb at %s: %w", dbPath, err)
			}
			results = append(results, sourceData{
				path: dbPath,
				rows: rowsFromDb,
			})
		}
	}

	return results, nil
}

func queryLeveldb(ctx context.Context, path string) ([]map[string][]byte, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	// If Chrome is open, we won't be able to open the db. So, copy it to a temporary location first.
	tempDbCopyLocation, err := indexeddb.CopyLeveldb(ctx, path)
	if err != nil {
		if tempDbCopyLocation != "" {
			_ = os.RemoveAll(tempDbCopyLocation)
		}
		return nil, fmt.Errorf("unable to copy db: %w", err)
	}
	// The copy was successful -- make sure we clean it up after we're done
	defer os.RemoveAll(tempDbCopyLocation)

	db, err := indexeddb.OpenLeveldb(tempDbCopyLocation)
	if err != nil {
		return nil, fmt.Errorf("opening leveldb: %w", err)
	}
	defer db.Close()

	rowsFromDb := make([]map[string][]byte, 0)
	iter := db.NewIterator(nil, nil)
	for iter.Next() {
		rowsFromDb = append(rowsFromDb, map[string][]byte{
			"key":   iter.Key(),
			"value": iter.Value(),
		})
	}
	iter.Release()
	if err := iter.Error(); err != nil {
		return rowsFromDb, fmt.Errorf("iterator error: %w", err)
	}

	return rowsFromDb, nil
}
