package launcher_db

import (
	"context"
	"fmt"

	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/osquery/osquery-go/plugin/table"
)

// TablePlugin provides an osquery table plugin that exposes data found in the server_provided_data launcher.db bucket.
// This data is intended to be updated by the control server.
func TablePlugin(tableName string, iterator types.Iterator) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("key"),
		table.TextColumn("value"),
	}

	return table.NewPlugin(tableName, columns, generateServerDataTable(tableName, iterator))
}

func generateServerDataTable(tableName string, iterator types.Iterator) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return dbKeyValueRows(tableName, iterator)
	}
}

func dbKeyValueRows(tableName string, iterator types.Iterator) ([]map[string]string, error) {
	results := make([]map[string]string, 0)

	if err := iterator.ForEach(func(k, v []byte) error {
		results = append(results, map[string]string{
			"key":   string(k),
			"value": string(v),
		})
		return nil
	}); err != nil {
		return nil, fmt.Errorf("could not fetch data from '%s' table: %w", tableName, err)
	}

	return results, nil
}
