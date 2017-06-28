// +build darwin

package osquery

import (
	"context"
	"strings"

	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

/*
Spotlight returns a macOS spotlight table
Example Query:
	SELECT uid, f.path FROM file
	AS f JOIN spotlight ON spotlight.path = f.path
	AND spotlight.query = "kMDItemKint = 'Agile Keychain'";
*/
func Spotlight() *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("query"),
		table.TextColumn("path"),
	}
	return table.NewPlugin("spotlight", columns, generateSpotlight)
}

func generateSpotlight(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	q, ok := queryContext.Constraints["query"]
	if !ok || len(q.Constraints) == 0 {
		return nil, errors.New("The spotlight table requires that you specify a constraint WHERE query =")
	}
	where := q.Constraints[0].Expression
	var query []string
	if strings.Contains(where, "-") {
		query = strings.Split(where, " ")
	} else {
		query = []string{where}
	}
	lines, err := mdfind(query...)
	if err != nil {
		return nil, errors.Wrap(err, "call mdfind")
	}
	var resp []map[string]string
	for _, line := range lines {
		m := make(map[string]string, 2)
		m["query"] = where
		m["path"] = line
		resp = append(resp, m)
	}
	return resp, nil
}
