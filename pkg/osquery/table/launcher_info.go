package table

import (
	"context"
	"runtime"

	"github.com/kolide/kit/version"
	"github.com/kolide/osquery-go/plugin/table"
)

func LauncherInfoTable() *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("branch"),
		table.TextColumn("build_date"),
		table.TextColumn("build_user"),
		table.TextColumn("go_version"),
		table.TextColumn("goarch"),
		table.TextColumn("goos"),
		table.TextColumn("revision"),
		table.TextColumn("version"),
	}
	return table.NewPlugin("kolide_launcher_info", columns, generateLauncherInfoTable())
}

func generateLauncherInfoTable() table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		results := []map[string]string{
			map[string]string{
				"branch":     version.Version().Branch,
				"build_date": version.Version().BuildDate,
				"build_user": version.Version().BuildUser,
				"go_version": runtime.Version(),
				"goarch":     runtime.GOARCH,
				"goos":       runtime.GOOS,
				"revision":   version.Version().Revision,
				"version":    version.Version().Version,
			},
		}

		return results, nil
	}
}
