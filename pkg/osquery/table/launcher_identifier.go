package table

import (
	"context"

	"github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/osquery-go/plugin/table"
	"go.etcd.io/bbolt"
)

func LauncherIdentifierTable(db *bbolt.DB) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("identifier"),
	}
	return table.NewPlugin("kolide_launcher_identifier", columns, generateLauncherIdentifier(db))
}

func generateLauncherIdentifier(db *bbolt.DB) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		identifier, err := osquery.IdentifierFromDB(db)
		if err != nil {
			return nil, err
		}
		results := []map[string]string{
			map[string]string{
				"identifier": identifier,
			},
		}

		return results, nil
	}
}
