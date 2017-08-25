package osquery

import (
	"context"

	"github.com/kolide/osquery-go/plugin/table"
)

func PersonalArtifacts() *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("type"),
		table.TextColumn("value"),
	}
	return table.NewPlugin("personal_artifacts", columns, generatePersonalArtifacts)
}

func generatePersonalArtifacts(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	return []map[string]string{
		map[string]string{
			"type":  "email",
			"value": "mike@kolide.co",
		},
		map[string]string{
			"type":  "email",
			"value": "mike@arpaia.co",
		},
	}, nil
}
