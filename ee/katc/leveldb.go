package katc

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/kolide/launcher/ee/indexeddb"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/table"
)

const (
	keyColumnName   = "key"
	valueColumnName = "value"
)

var leveldbExpectedColumns = map[string]struct{}{
	keyColumnName:   {},
	valueColumnName: {},
	pathColumnName:  {},
}

func validateLeveldbTableColumns(columns []table.ColumnDefinition) error {
	for _, c := range columns {
		if _, ok := leveldbExpectedColumns[c.Name]; !ok {
			return fmt.Errorf("unsupported column %s for leveldb table", c.Name)
		}
	}

	return nil
}

// leveldbData is the dataFunc for plain LevelDB databases without additional encoding
// or nesting. IndexedDB databases leveraging LevelDB should use `indexeddbLeveldbData`
// instead. If set, the query is a comma-separated allowlist of keys to return; if empty,
// all keys are returned.
func leveldbData(ctx context.Context, slogger *slog.Logger, sourcePaths []string, query string, queryContext table.QueryContext) ([]sourceData, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	// Pull out path constraints from the query against the KATC table, to avoid querying more leveldb files than we need to.
	pathConstraintsFromQuery := getPathConstraint(queryContext)

	allowedKeyMap := extractLeveldbQueryTargets(query)

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

			rowsFromDb, err := queryLeveldb(ctx, dbPath, allowedKeyMap)
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

func queryLeveldb(ctx context.Context, path string, allowedKeyMap map[string]struct{}) ([]map[string][]byte, error) {
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
		// We have to copy over the key/value to prevent their data from
		// being overwritten on subsequent iterations.
		k := make([]byte, len(iter.Key()))
		copy(k, iter.Key())
		if len(allowedKeyMap) > 0 {
			if _, ok := allowedKeyMap[string(k)]; !ok {
				// Key is not allowlisted -- skip it
				continue
			}
		}
		v := make([]byte, len(iter.Value()))
		copy(v, iter.Value())
		rowsFromDb = append(rowsFromDb, map[string][]byte{
			"key":   k,
			"value": v,
		})
	}
	iter.Release()
	if err := iter.Error(); err != nil {
		return rowsFromDb, fmt.Errorf("iterator error: %w", err)
	}

	return rowsFromDb, nil
}

func extractLeveldbQueryTargets(query string) map[string]struct{} {
	allowedKeyMap := make(map[string]struct{})

	for _, allowedKey := range strings.Split(query, ",") {
		if len(allowedKey) == 0 {
			continue
		}
		allowedKeyMap[allowedKey] = struct{}{}
	}

	return allowedKeyMap
}
