package osquery_instance_history

import (
	"context"
	"errors"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
)

func TablePlugin(k types.Knapsack, slogger *slog.Logger) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("registration_id"),
		table.TextColumn("instance_run_id"),
		table.TextColumn("start_time"),
		table.TextColumn("connect_time"),
		table.TextColumn("exit_time"),
		table.TextColumn("hostname"),
		table.TextColumn("instance_id"),
		table.TextColumn("version"),
		table.TextColumn("errors"),
	}
	return tablewrapper.New(k, slogger, "kolide_launcher_osquery_instance_history", columns, generate(k))
}

func generate(k types.Knapsack) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		_, span := observability.StartSpan(ctx, "table_name", "kolide_launcher_osquery_instance_history")
		defer span.End()

		osqHistory := k.OsqueryHistory()
		if osqHistory == nil {
			return nil, errors.New("osquery history is unavailable")
		}

		results, err := osqHistory.GetHistory()
		if err != nil {
			return nil, err
		}

		return results, nil
	}
}
