// +build windows

package wmitable

import (
	"context"
	"strings"
	"time"

	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/kolide/launcher/pkg/wmi"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

const allowedCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-"

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
		table.TextColumn("namespace"),
		table.TextColumn("class"),
		table.TextColumn("properties"),
		table.TextColumn("whereclause"),
	}

	t := &Table{
		client: client,
		logger: level.NewFilter(logger),
	}

	return table.NewPlugin("kolide_wmi", columns, t.generate)

}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	classes := tablehelpers.GetConstraints(queryContext, "class", tablehelpers.WithAllowedCharacters(allowedCharacters))
	if len(classes) == 0 {
		return nil, errors.New("The kolide_wmi table requires a wmi class")
	}

	properties := tablehelpers.GetConstraints(queryContext, "properties", tablehelpers.WithAllowedCharacters(allowedCharacters+`,`))
	if len(properties) == 0 {
		return nil, errors.New("The kolide_wmi table requires wmi properties")
	}

	// Get the list of namespaces to query. If not specified, that's
	// okay too, default to ""
	namespaces := tablehelpers.GetConstraints(queryContext, "namespace",
		tablehelpers.WithDefaults(""),
		tablehelpers.WithAllowedCharacters(allowedCharacters+`\`),
	)

	// Any whereclauses? These are not required
	whereClauses := tablehelpers.GetConstraints(queryContext, "whereclause",
		tablehelpers.WithDefaults(""),
		tablehelpers.WithAllowedCharacters(allowedCharacters+`:\= '".`),
	)

	flattenQueries := tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults(""))

	for _, class := range classes {
		for _, rawProperties := range properties {
			properties := strings.Split(rawProperties, ",")
			if len(properties) == 0 {
				continue
			}
			for _, ns := range namespaces {
				for _, whereClause := range whereClauses {
					// Set a timeout in case wmi hangs
					ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
					defer cancel()

					// Add a logger in
					ctx = ctxlog.NewContext(ctx, t.logger)

					wmiResults, err := wmi.Query(ctx, class, properties, wmi.ConnectUseMaxWait(), wmi.ConnectNamespace(ns), wmi.WithWhere(whereClause))
					if err != nil {
						level.Info(t.logger).Log(
							"msg", "wmi query failure",
							"err", err,
							"class", class,
							"properties", rawProperties,
							"namespace", ns,
							"whereClause", whereClause,
						)
						continue
					}

					for _, dataQuery := range flattenQueries {
						results = append(results, t.flattenRowsFromWmi(dataQuery, wmiResults, class, rawProperties, ns, whereClause)...)
					}
				}
			}
		}
	}

	return results, nil
}

func (t *Table) flattenRowsFromWmi(dataQuery string, wmiResults []map[string]interface{}, wmiClass, wmiProperties, wmiNamespace, whereClause string) []map[string]string {
	flattenOpts := []dataflatten.FlattenOpts{}

	if dataQuery != "" {
		flattenOpts = append(flattenOpts, dataflatten.WithQuery(strings.Split(dataQuery, "/")))
	}

	if t.logger != nil {
		flattenOpts = append(flattenOpts, dataflatten.WithLogger(level.NewFilter(t.logger, level.AllowInfo())))
	}

	// wmi.Query returns []map[string]interface{}, but dataflatten
	// wants it as []interface{}. So let's whomp it.
	resultsCasted := make([]interface{}, len(wmiResults))
	for i, r := range wmiResults {
		resultsCasted[i] = r
	}

	flatData, err := dataflatten.Flatten(resultsCasted, flattenOpts...)
	if err != nil {
		level.Info(t.logger).Log("msg", "failure flattening output", "err", err)
		return nil
	}

	var results []map[string]string

	for _, row := range flatData {
		p, k := row.ParentKey("/")

		res := map[string]string{
			"fullkey":     row.StringPath("/"),
			"parent":      p,
			"key":         k,
			"value":       row.Value,
			"query":       dataQuery,
			"class":       wmiClass,
			"properties":  wmiProperties,
			"namespace":   wmiNamespace,
			"whereclause": whereClause,
		}
		results = append(results, res)
	}
	return results
}
