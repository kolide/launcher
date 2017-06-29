package osquery

import (
	"context"

	"github.com/kolide/osquery-go/plugin/table"
)

func BestPractices() *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("password_required_from_screensaver"),
	}
	return table.NewPlugin("kolide_best_practices", columns, generateBestPractices)
}

func generateBestPractices(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	results := []map[string]string{
		map[string]string{
			"password_required_from_screensaver": "true",
		},
	}
	return results, nil
}
