package table

import (
	"context"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go/plugin/table"
)

const launcherAutoupdateConfigTableName = "kolide_launcher_autoupdate_config"

func LauncherAutoupdateConfigTable(slogger *slog.Logger, flags types.Flags) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("autoupdate"),
		table.TextColumn("mirror_server_url"),
		table.TextColumn("tuf_server_url"),
		table.TextColumn("autoupdate_interval"),
		table.TextColumn("update_channel"),
	}

	return tablewrapper.New(flags, slogger, launcherAutoupdateConfigTableName, columns, generateLauncherAutoupdateConfigTable(flags))
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
		_, span := observability.StartSpan(ctx, "table_name", launcherAutoupdateConfigTableName)
		defer span.End()

		return []map[string]string{
			{
				"autoupdate":          boolToString(flags.Autoupdate()),
				"mirror_server_url":   flags.MirrorServerURL(),
				"tuf_server_url":      flags.TufServerURL(),
				"autoupdate_interval": flags.AutoupdateInterval().String(),
				"update_channel":      flags.UpdateChannel(),
			},
		}, nil
	}
}
