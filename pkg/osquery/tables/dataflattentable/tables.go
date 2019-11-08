package dataflattentable

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

type DataSourceType int

const (
	PlistType DataSourceType = iota + 1
	JsonType
)

type Table struct {
	client    *osquery.ExtensionManagerClient
	logger    log.Logger
	dataFunc  func(string, ...dataflatten.FlattenOpts) ([]dataflatten.Row, error)
	tableName string
}

func TablePlugin(client *osquery.ExtensionManagerClient, logger log.Logger, dataSourceType DataSourceType) *table.Plugin {

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

	switch dataSourceType {
	case PlistType:
		t.dataFunc = dataflatten.PlistFile
		t.tableName = "kolide_plist"
	case JsonType:
		t.dataFunc = dataflatten.JsonFile
		t.tableName = "kolide_json"
	default:
		panic("Unknown data source type")
	}

	return table.NewPlugin(t.tableName, columns, t.generate)

}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	flattenOpts := []dataflatten.FlattenOpts{}

	if t.logger != nil {
		flattenOpts = append(flattenOpts, dataflatten.WithLogger(t.logger))
	}

	var results []map[string]string

	pathQ, ok := queryContext.Constraints["path"]
	if !ok || len(pathQ.Constraints) == 0 {
		return results, errors.Errorf("The %s table requires that you specify a single constraint for path", t.tableName)
	}
	for _, pathConstraint := range pathQ.Constraints {

		// We take globs in via the sql %, but glob needs *. So convert.
		filePaths, err := filepath.Glob(strings.ReplaceAll(pathConstraint.Expression, `%`, `*`))
		if err != nil {
			return results, errors.Wrap(err, "bad glob")
		}

		for _, filePath := range filePaths {

			if q, ok := queryContext.Constraints["query"]; ok && len(q.Constraints) != 0 {

				for _, constraint := range q.Constraints {
					dataQuery := constraint.Expression

					data, err := t.dataFunc(filePath,
						append(flattenOpts, dataflatten.WithQuery(strings.Split(dataQuery, "/")))...)
					if err != nil {
						level.Info(t.logger).Log("msg", "failure parsing file", "file", filePath)
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
							"query":   dataQuery,
						}
						results = append(results, res)
					}
				}
			} else {
				data, err := t.dataFunc(filePath, flattenOpts...)
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
	}

	return results, nil
}
