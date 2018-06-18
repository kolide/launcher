package table

import (
	"context"

	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

func Airdrop(client *osquery.ExtensionManagerClient) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("username"),
		table.TextColumn("discover_by"),
	}
	t := &airdropTable{client: client}
	return table.NewPlugin("kolide_airdrop_preferences", columns, t.generateAirdrop)
}

type airdropTable struct {
	client      *osquery.ExtensionManagerClient
	primaryUser string
}

func (t *airdropTable) generateAirdrop(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	if t.primaryUser == "" {
		username, err := queryPrimaryUser(t.client)
		if err != nil {
			return nil, errors.Wrap(err, "query primary user for airdrop table")
		}
		t.primaryUser = username
	}

	discover := fromCFPlistRef(copyValue("DiscoverableMode", "com.apple.sharingd", t.primaryUser)).(string)
	return []map[string]string{
		map[string]string{
			"username":    t.primaryUser,
			"discover_by": discover,
		},
	}, nil
}

func queryPrimaryUser(client *osquery.ExtensionManagerClient) (string, error) {
	query := `select username from last where username not in ('', 'root') group by username order by count(username) desc limit 1`
	row, err := client.QueryRow(query)
	if err != nil {
		return "", errors.Wrap(err, "querying for primaryUser version")
	}
	var username string
	if val, ok := row["username"]; ok {
		username = val
	}
	return username, nil
}
