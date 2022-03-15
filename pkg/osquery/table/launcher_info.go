package table

import (
	"context"
	"runtime"

	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/osquery/osquery-go/plugin/table"
	"go.etcd.io/bbolt"
)

func LauncherInfoTable(db *bbolt.DB) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("branch"),
		table.TextColumn("build_date"),
		table.TextColumn("build_user"),
		table.TextColumn("go_version"),
		table.TextColumn("goarch"),
		table.TextColumn("goos"),
		table.TextColumn("revision"),
		table.TextColumn("version"),
		table.TextColumn("identifier"),
	}
	return table.NewPlugin("kolide_launcher_info", columns, generateLauncherInfoTable(db))
}

func generateLauncherInfoTable(db *bbolt.DB) table.GenerateFunc {

	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		identifier, err := osquery.IdentifierFromDB(db)

		if err != nil {
			return nil, err
		}

		results := []map[string]string{
			{
				"branch":     version.Version().Branch,
				"build_date": version.Version().BuildDate,
				"build_user": version.Version().BuildUser,
				"go_version": runtime.Version(),
				"goarch":     runtime.GOARCH,
				"goos":       runtime.GOOS,
				"revision":   version.Version().Revision,
				"version":    version.Version().Version,
				"identifier": identifier,
			},
		}

		return results, nil
	}
}

// func LauncherIdentifierTable(db *bbolt.DB) *table.Plugin {
// 	columns := []table.ColumnDefinition{
// 		table.TextColumn("identifier"),
// 	}
// 	return table.NewPlugin("kolide_launcher_identifier", columns, generateLauncherIdentifier(db))
// }

// func generateLauncherIdentifier(db *bbolt.DB) table.GenerateFunc {
// 	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
// 		identifier, err := osquery.IdentifierFromDB(db)
// 		if err != nil {
// 			return nil, err
// 		}
// 		results := []map[string]string{
// 			map[string]string{
// 				"identifier": identifier,
// 			},
// 		}

// 		return results, nil
// 	}
// }
