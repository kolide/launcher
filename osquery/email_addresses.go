package osquery

import (
	"context"

	osquery "github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
)

func EmailAddresses(client *osquery.ExtensionManagerClient) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("email"),
		table.TextColumn("domain"),
	}
	return table.NewPlugin("email_addresses", columns, generateEmailAddresses(client))
}

func generateEmailAddresses(client *osquery.ExtensionManagerClient) table.GenerateFunc {
	return func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{
			map[string]string{
				"email":  "mike@kolide.co",
				"domain": "kolide.co",
			},
			map[string]string{
				"email":  "mike@arpaia.co",
				"domain": "arpaia.co",
			},
		}, nil
	}
}
