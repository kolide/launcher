package osquery_instance_history

import (
	"context"

	"github.com/kolide/launcher/pkg/osquery/runtime/history"
	"github.com/osquery/osquery-go/plugin/table"
)

func TablePlugin() *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("start_time"),
		table.TextColumn("connect_time"),
		table.TextColumn("exit_time"),
		table.TextColumn("hostname"),
		table.TextColumn("instance_id"),
		table.TextColumn("version"),
		table.TextColumn("errors"),
	}
	return table.NewPlugin("kolide_launcher_osquery_instance_history", columns, generate())
}

func generate() table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		results := []map[string]string{}

		history, err := history.GetHistory()
		if err != nil {
			return nil, err
		}

		for _, instance := range history {

			results = append(results, map[string]string{
				"start_time":   instance.StartTime,
				"connect_time": instance.ConnectTime,
				"exit_time":    instance.ExitTime,
				"instance_id":  instance.InstanceId,
				"version":      instance.Version,
				"hostname":     instance.Hostname,
				"errors":       instance.Error,
			})
		}

		return results, nil
	}
}
