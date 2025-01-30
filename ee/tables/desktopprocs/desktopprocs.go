package desktopprocs

import (
	"context"
	"fmt"

	"github.com/kolide/launcher/ee/desktop/runner"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/table"
)

func TablePlugin() *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("uid"),
		table.TextColumn("pid"),
		table.TextColumn("start_time"),
		table.TextColumn("last_health_check"),
	}
	return table.NewPlugin("kolide_desktop_procs", columns, generate())
}

func generate() table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		_, span := traces.StartSpan(ctx, "table_name", "kolide_desktop_procs")
		defer span.End()

		results := []map[string]string{}

		for k, v := range runner.InstanceDesktopProcessRecords() {
			results = append(results, map[string]string{
				"uid":               k,
				"pid":               fmt.Sprint(v.Process.Pid),
				"start_time":        fmt.Sprint(v.StartTime),
				"last_health_check": fmt.Sprint(v.LastHealthCheck),
			})
		}

		return results, nil
	}
}
