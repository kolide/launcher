package osquery_instance_history

import (
	"context"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/kolide/launcher/pkg/osquery/runtime/history"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/table"
)

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
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
	return tablewrapper.New(flags, slogger, "kolide_launcher_osquery_instance_history", columns, generate())
}

func generate() table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		_, span := traces.StartSpan(ctx, "table_name", "kolide_launcher_osquery_instance_history")
		defer span.End()

		results := []map[string]string{}

		history, err := history.GetHistory()
		if err != nil {
			return nil, err
		}

		for _, instance := range history {

			results = append(results, map[string]string{
				"registration_id": instance.RegistrationId,
				"instance_run_id": instance.RunId,
				"start_time":      instance.StartTime,
				"connect_time":    instance.ConnectTime,
				"exit_time":       instance.ExitTime,
				"instance_id":     instance.InstanceId,
				"version":         instance.Version,
				"hostname":        instance.Hostname,
				"errors":          instance.Error,
			})
		}

		return results, nil
	}
}
