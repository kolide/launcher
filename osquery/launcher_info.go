package osquery

import (
	"context"

	"github.com/kolide/kit/version"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
)

func LauncherInfo(client *osquery.ExtensionManagerClient) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("version"),
		table.TextColumn("go_version"),
		table.TextColumn("branch"),
		table.TextColumn("revision"),
		table.TextColumn("build_date"),
		table.TextColumn("build_user"),
	}
	return table.NewPlugin("kolide_launcher_info", columns, generateLauncherInfo(client))
}

func generateLauncherInfo(client *osquery.ExtensionManagerClient) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		results := []map[string]string{
			map[string]string{
				"version":    version.Version().Version,
				"go_version": version.Version().GoVersion,
				"branch":     version.Version().Branch,
				"revision":   version.Version().Revision,
				"build_date": version.Version().BuildDate,
				"build_user": version.Version().BuildUser,
			},
		}

		return results, nil
	}
}
