package launcher_db

import (
	"context"
	"fmt"

	"github.com/osquery/osquery-go/plugin/table"

	"go.etcd.io/bbolt"
)

// TablePlugin provides an osquery table plugin that exposes data found in the server_provided_data launcher.db bucket.
// This data is intended to be updated by the control server.
func TablePlugin(db *bbolt.DB, tableName, bucketName string) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("key"),
		table.TextColumn("value"),
	}

	return table.NewPlugin(tableName, columns, generateLauncherDbTable(db, bucketName))
}

func generateLauncherDbTable(db *bbolt.DB, bucket string) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return dbKeyValueRows(bucket, db)
	}
}

func dbKeyValueRows(bucketName string, db *bbolt.DB) ([]map[string]string, error) {
	results := make([]map[string]string, 0)

	if err := db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b == nil {
			return fmt.Errorf("%s bucket not found", bucketName)
		}

		b.ForEach(func(k, v []byte) error {
			results = append(results, map[string]string{
				"key":   string(k),
				"value": string(v),
			})
			return nil
		})
		return nil
	}); err != nil {
		return nil, fmt.Errorf("fetching data: %w", err)
	}

	return results, nil
}
