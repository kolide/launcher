package osqtable

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
	// cache the primary user if unset
	if t.primaryUser == "" {
		username, err := queryPrimaryUser(t.client)
		if err != nil {
			return nil, errors.Wrap(err, "query primary user for airdrop table")
		}
		t.primaryUser = username
	}

	// use the username from the query context if provide, otherwise default to primary user
	var username string
	q, ok := queryContext.Constraints["username"]
	if ok && len(q.Constraints) != 0 {
		username = q.Constraints[0].Expression
	} else {
		username = t.primaryUser
	}

	discover := "Unknown"
	if val, ok := copyPreferenceValue("DiscoverableMode", "com.apple.sharingd", username).(string); ok {
		discover = val
	}
	return []map[string]string{
		map[string]string{
			"username":    username,
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
