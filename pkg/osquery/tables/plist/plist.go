package plist

import (
	"context"
	"fmt"
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
	flattenOpts := []dataflatten.FlattenOpts{}

	if t.logger != nil {
		flattenOpts = append(flattenOpts, dataflatten.WithLogger(t.logger))
	}

	var results []map[string]string

	pathQ, ok := queryContext.Constraints["path"]
	if !ok || len(pathQ.Constraints) == 0 {
		return results, errors.New("The kolide_plist table requires that you specify a single constraint for path")
	}
	for _, pathConstraint := range pathQ.Constraints {

		filePath := pathConstraint.Expression

		if q, ok := queryContext.Constraints["query"]; ok && len(q.Constraints) != 0 {

			for _, constraint := range q.Constraints {
				plistQuery := constraint.Expression

				data, err := dataflatten.PlistFile(filePath,
					append(flattenOpts, dataflatten.WithQuery(strings.Split(plistQuery, "/")))...)
				if err != nil {
					fmt.Println("parse failjure")
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
			data, err := dataflatten.PlistFile(filePath, flattenOpts...)
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
	}

	return results, nil
}
