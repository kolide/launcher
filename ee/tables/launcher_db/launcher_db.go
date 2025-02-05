package launcher_db

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/table"
)

// TablePlugin provides an osquery table plugin that exposes data found in the server_provided_data launcher.db bucket.
// This data is intended to be updated by the control server.
func TablePlugin(flags types.Flags, slogger *slog.Logger, tableName string, iterator types.Iterator) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("key"),
		table.TextColumn("value"),
	}

	return tablewrapper.New(flags, slogger, tableName, columns, generateServerDataTable(tableName, iterator))
}

func generateServerDataTable(tableName string, iterator types.Iterator) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		ctx, span := traces.StartSpan(ctx, "table_name", tableName)
		defer span.End()

		return dbKeyValueRows(ctx, tableName, iterator)
	}
}

func dbKeyValueRows(ctx context.Context, tableName string, iterator types.Iterator) ([]map[string]string, error) {
	_, span := traces.StartSpan(ctx)
	defer span.End()

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
