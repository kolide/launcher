package table

import (
	"context"

	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/osquery/osquery-go/plugin/table"
)

func LauncherConfigTable(store types.Getter) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("config"),
	}
	return table.NewPlugin("kolide_launcher_config", columns, generateLauncherConfig(db))
}

func generateLauncherConfig(store types.Getter) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		config, err := osquery.Config(store)
		if err != nil {
			return nil, err
		}
		results := []map[string]string{
			{
				"config": config,
			},
		}

		return results, nil
	}
}
