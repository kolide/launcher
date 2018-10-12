package table

import (
	"github.com/kolide/osquery-go"
	"github.com/pkg/errors"
)

func getPrimaryUser(client *osquery.ExtensionManagerClient) (string, error) {
	query := `select username from last where username not in ('', 'root') group by username order by count(username) desc limit 1`
	row, err := client.QueryRow(query)
	if err != nil {
		return "", errors.Wrap(err, "querying for primaryUser version")
	}
	return row["username"], nil
}
