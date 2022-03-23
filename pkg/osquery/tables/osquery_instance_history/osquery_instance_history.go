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

		for _, v := range history {

			errStr := ""
			if v.Error != nil {
				errStr = v.Error.Error()
			}

			results = append(results, map[string]string{
				"start_time":   v.StartTime,
				"connect_time": v.ConnectTime,
				"exit_time":    v.ExitTime,
				"instance_id":  v.InstanceId,
				"version":      v.Version,
				"hostname":     v.Hostname,
				"errors":       errStr,
			})
		}

		return results, nil
	}
}
