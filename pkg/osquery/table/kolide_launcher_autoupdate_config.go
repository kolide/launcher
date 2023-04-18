package table

import (
	"context"

	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/osquery/osquery-go/plugin/table"
)

const launcherAutoupdateConfigTableName = "kolide_launcher_autoupdate_config"

func LauncherAutoupdateConfigTable(flags types.Flags) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("autoupdate"),
		table.TextColumn("notary_server_url"),
		table.TextColumn("mirror_server_url"),
		table.TextColumn("tuf_server_url"),
		table.TextColumn("autoupdate_interval"),
		table.TextColumn("update_channel"),
	}

	return table.NewPlugin(launcherAutoupdateConfigTableName, columns, generateLauncherAutoupdateConfigTable(flags))
}

func boolToString(in bool) string {
	if in {
		return "1"
	} else {
		return "0"
	}
}

func generateLauncherAutoupdateConfigTable(flags types.Flags) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{
			{
				"autoupdate":          boolToString(flags.Autoupdate()),
				"notary_server_url":   flags.NotaryServerURL(),
				"mirror_server_url":   flags.MirrorServerURL(),
				"tuf_server_url":      flags.TufServerURL(),
				"autoupdate_interval": flags.AutoupdateInterval().String(),
				"update_channel":      flags.UpdateChannel(),
			},
		}, nil
	}
}
