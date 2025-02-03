//go:build windows
// +build windows

package wmitable

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/kolide/launcher/ee/wmi"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/table"
)

const allowedCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-"

type Table struct {
	slogger *slog.Logger
}

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {

	columns := dataflattentable.Columns(
		table.TextColumn("namespace"),
		table.TextColumn("class"),
		table.TextColumn("properties"),
		table.TextColumn("whereclause"),
	)

	t := &Table{
		slogger: slogger.With("table", "kolide_wmi"),
	}

	return tablewrapper.New(flags, slogger, "kolide_wmi", columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := traces.StartSpan(ctx, "table_name", "kolide_wmi")
	defer span.End()

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

	flattenQueries := tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*"))

	for _, class := range classes {
		for _, rawProperties := range properties {
			properties := strings.Split(rawProperties, ",")
			if len(properties) == 0 {
				continue
			}
			for _, ns := range namespaces {
				// The namespace argument uses a bare
				// backslash, not a doubled one. But,
				// it's common to double backslashes
				// to escape them through quoting
				// blocks. We can collapse them it
				// down here, and create a small ux
				// improvement.
				ns = strings.ReplaceAll(ns, `\\`, `\`)

				for _, whereClause := range whereClauses {
					wmiResults, err := t.runQuery(ctx, class, properties, ns, whereClause)
					if err != nil {
						t.slogger.Log(ctx, slog.LevelInfo,
							"wmi query failure",
							"err", err,
							"class", class,
							"properties", rawProperties,
							"namespace", ns,
							"where_clause", whereClause,
						)
						continue
					}

					for _, dataQuery := range flattenQueries {
						results = append(results, t.flattenRowsFromWmi(ctx, dataQuery, wmiResults, class, rawProperties, ns, whereClause)...)
					}
				}
			}
		}
	}

	return results, nil
}

func (t *Table) runQuery(ctx context.Context, class string, properties []string, ns string, whereClause string) ([]map[string]interface{}, error) {
	ctx, span := traces.StartSpan(ctx, "class", class)
	defer span.End()

	// Set a timeout in case wmi hangs
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	return wmi.Query(ctx, t.slogger, class, properties, wmi.ConnectUseMaxWait(), wmi.ConnectNamespace(ns), wmi.WithWhere(whereClause))
}

func (t *Table) flattenRowsFromWmi(ctx context.Context, dataQuery string, wmiResults []map[string]interface{}, wmiClass, wmiProperties, wmiNamespace, whereClause string) []map[string]string {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	flattenOpts := []dataflatten.FlattenOpts{
		dataflatten.WithSlogger(t.slogger),
		dataflatten.WithQuery(strings.Split(dataQuery, "/")),
	}

	// wmi.Query returns []map[string]interface{}, but dataflatten
	// wants it as []interface{}. So let's whomp it.
	resultsCasted := make([]interface{}, len(wmiResults))
	for i, r := range wmiResults {
		resultsCasted[i] = r
	}

	flatData, err := dataflatten.Flatten(resultsCasted, flattenOpts...)
	if err != nil {
		t.slogger.Log(ctx, slog.LevelInfo,
			"failure flattening output",
			"err", err,
		)
		return nil
	}

	rowData := map[string]string{
		"class":       wmiClass,
		"properties":  wmiProperties,
		"namespace":   wmiNamespace,
		"whereclause": whereClause,
	}

	return dataflattentable.ToMap(flatData, dataQuery, rowData)
}
