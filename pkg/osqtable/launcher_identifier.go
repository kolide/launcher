package osqtable

import (
	"context"

	"github.com/boltdb/bolt"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/kolide/osquery-go/plugin/table"
)

func LauncherIdentifierTable(db *bolt.DB) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("identifier"),
	}
	return table.NewPlugin("kolide_launcher_identifier", columns, generateLauncherIdentifier(db))
}

func generateLauncherIdentifier(db *bolt.DB) table.GenerateFunc {
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
