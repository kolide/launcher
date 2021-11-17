package table

import (
	"context"

	"github.com/kolide/launcher/pkg/launcher"
	"github.com/osquery/osquery-go/plugin/table"
)

const launcherAutoupdateConfigTableName = "kolide_launcher_autoupdate_config"

func LauncherAutoupdateConfigTable(opts *launcher.Options) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("autoupdate"),
		table.TextColumn("notary_server_url"),
		table.TextColumn("mirror_server_url"),
		table.TextColumn("autoupdate_interval"),
		table.TextColumn("update_channel"),
	}

	return table.NewPlugin(launcherAutoupdateConfigTableName, columns, generateLauncherAutoupdateConfigTable(opts))
}

func boolToString(in bool) string {
	if in {
		return "1"
	} else {
		return "0"
	}
}

func generateLauncherAutoupdateConfigTable(opts *launcher.Options) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{
			map[string]string{
				"autoupdate":          boolToString(opts.Autoupdate),
				"notary_server_url":   opts.NotaryServerURL,
				"mirror_server_url":   opts.MirrorServerURL,
				"autoupdate_interval": opts.AutoupdateInterval.String(),
				"update_channel":      string(opts.UpdateChannel),
			},
		}, nil
	}
}
