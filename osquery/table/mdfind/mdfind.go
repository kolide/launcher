// +build darwin

/*
package mdfind implements an osquery table for the macOS spotlight search.

Example Query:
	SELECT uid, f.path FROM file
	AS f JOIN mdfind ON mdfind.path = f.path
	AND mdfind.query = "kMDItemKint = 'Agile Keychain'";
*/
package mdfind

import (
	"context"
	"strings"

	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

func NewTable(name string) *table.Plugin {
	return table.NewPlugin(name, Columns(), Generate)
}

func Columns() []table.ColumnDefinition {
	return []table.ColumnDefinition{
		table.TextColumn("query"),
		table.TextColumn("path"),
	}
}

func Generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	where := queryContext.Constraints["query"].Constraints[0].Expression
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
