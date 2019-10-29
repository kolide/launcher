package plist

import (
	"context"
	"io/ioutil"
	"os"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

type Table struct {
	client *osquery.ExtensionManagerClient
	logger log.Logger
}

func TablePlugin(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {

	columns := []table.ColumnDefinition{
		table.TextColumn("path"),
		table.TextColumn("fullkey"),
		table.TextColumn("parent"),
		table.TextColumn("key"),
		table.TextColumn("value"),
		table.TextColumn("query"),
	}

	t := &Table{
		client: client,
		logger: logger,
	}

	return table.NewPlugin("kolide_plist", columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	q, ok := queryContext.Constraints["path"]
	if !ok || len(q.Constraints) == 0 {
		return results, errors.New("The kolide_plist table requires that you specify a single constraint for path")
	}
	if len(q.Constraints) > 1 {
		return results, errors.New("The kolide_plist table requires that you specify a single constraint for path")
	}

	filePath := q.Constraints[0].Expression

	file, err := os.Open(filePath)
	if err != nil {
		return results, errors.Wrap(err, "failed to open file")
	}

	fileBytes, err := ioutil.ReadAll(file)
	if err != nil {
		return results, errors.Wrap(err, "failed to read file")
	}

	if q, ok := queryContext.Constraints["query"]; ok {
		for _, constraint := range q.Constraints {
			plistQuery := constraint.Expression

			data, err := dataflatten.Plist(fileBytes, dataflatten.WithQuery(strings.Split(plistQuery, "/")))
			if err != nil {
				return results, errors.Wrap(err, "parsing data")
			}

			for _, row := range data {
				p, k := row.ParentKey("/")

				res := map[string]string{
					"path":    filePath,
					"fullkey": row.StringPath("/"),
					"parent":  p,
					"key":     k,
					"value":   row.Value,
					"query":   plistQuery,
				}
				results = append(results, res)
			}
		}
	} else {
		// no constraints
		data, err := dataflatten.Plist(fileBytes)
		if err != nil {
			return results, errors.Wrap(err, "parsing data")
		}
		for _, row := range data {
			p, k := row.ParentKey("/")

			res := map[string]string{
				"path":    filePath,
				"fullkey": row.StringPath("/"),
				"parent":  p,
				"key":     k,
				"value":   row.Value,
			}
			results = append(results, res)

		}
	}

	return results, nil
}
