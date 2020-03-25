// +build windows

package wmitable

import (
	"context"
	"log"
	"strings"

	"github.com/go-kit/kit/log/level"
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
		// dataflatten columns
		table.TextColumn("fullkey"),
		table.TextColumn("parent"),
		table.TextColumn("key"),
		table.TextColumn("value"),
		table.TextColumn("query"),

		// wmi columns
		table.TextColumn("class"),
		table.TextColumn("properties"),
	}

	t := &Table{
		client: client,
		logger: logger,
	}

	return table.NewPlugin("kolide_wmi", columns, t.generate)

}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	classQ, ok := queryContext.Constraints["class"]
	if !ok || len(queryQ.Constraints) == 0 {
		return nil, errors.New("The kolide_wmi table requires a wmi class")
	}

	propertiesQ, ok := queryContext.Constraints["properties"]
	if !ok || len(propertiesQ.Constraints) == 0 {
		// TODO: consider defaulting to "name" here?
		return nil, errors.New("The kolide_wmi table requires wmi properties")
	}

	for _, classConstraint := range classQ.Constraints {
		for _, propertiesConstraint := range propertiesQ.Constraints {
			properties := strings.Split(propertiesConstraint, ",")
			if len(properties) == 0 {
				continue
			}

			wmiResults, err := wmi.Query(ctx, classConstraint, properties)
			if err != nil {
				level.Info(t.logger).Log(
					"msg", "wmi query failure",
					"err", err,
					"class", classConstraint,
					"properties", propertiesConstraint,
				)
				continue
			}

			if q, ok := queryContext.Constraints["query"]; ok && len(q.Constraints) != 0 {
				for _, constraint := range q.Constraints {
					dataQuery := constraint.Expression
					results = append(results, t.flattenRowsFromWmi(dataQuery, wmiResults, classConstraint, propertiesConstraint)...)
				}
			} else {
				results = append(results, t.flattenRowsFromWmi("", wmiResults, classConstraint, propertiesConstraint)...)
			}

		}
	}

	return results, nil

}

func (t *Table) flattenRowsFromWmi(dataQuery string, wmiResults []map[string]interface{}, wmiClass, wmiProperties string) []map[string]string {
	flattenOpts := []dataflatten.FlattenOpts{}

	if dataQuery != "" {
		flattenOpts = append(flattenOpts, dataflatten.WithQuery(strings.Split(dataQuery, "/")))
	}

	if t.logger != nil {
		flattenOpts = append(flattenOpts, dataflatten.WithLogger(t.logger))
	}

	flatData, err := dataflatten.Flatten(wmiResults, flattenOpts...)
	if err != nil {
		level.Info(t.logger).Log("msg", "failure flattening output", "err", err)
		return nil
	}

	var results []map[string]string

	for _, row := range flatData {
		p, k := row.ParentKey("/")

		res := map[string]string{
			"fullkey":    row.StringPath("/"),
			"parent":     p,
			"key":        k,
			"value":      row.Value,
			"query":      dataQuery,
			"class":      wmiClass,
			"properties": wmiProperties,
		}
		results = append(results, res)
	}
	return results
}
