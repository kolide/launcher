package dataflattentable

import (
	"fmt"
	"maps"
	"strings"

	"github.com/kolide/launcher/v2/ee/dataflatten"
	"github.com/kolide/launcher/v2/ee/tables/tablehelpers"
	"github.com/osquery/osquery-go/plugin/table"
)

// ToMap is a helper function to convert Flatten output directly for
// consumption by osquery tables.
func ToMap(rows []dataflatten.Row, query string, prefilter string, rowData map[string]string) []map[string]string {
	results := make([]map[string]string, len(rows))

	for i, row := range rows {
		res := make(map[string]string, len(rowData)+5)
		maps.Copy(res, rowData)

		p, k := row.ParentKey("/")

		res["fullkey"] = row.StringPath("/")
		res["parent"] = p
		res["key"] = k
		res["value"] = row.Value
		res["query"] = query
		res["prefilter"] = prefilter

		results[i] = res
	}

	return results
}

// Columns returns the standard data flatten columns, plus whatever
// ones have been provided as additional. This is syntantic sugar for
// dataflatten based tables.
func Columns(additional ...table.ColumnDefinition) []table.ColumnDefinition {
	columns := []table.ColumnDefinition{
		table.TextColumn("fullkey"),
		table.TextColumn("parent"),
		table.TextColumn("key"),
		table.TextColumn("value"),
		table.TextColumn("query"),
		table.TextColumn("prefilter"),
	}

	return append(columns, additional...)
}

// ExtractPrefilterFromQuery retrieves and compiles the CEL given in the queryContext,
// if one is available. The returned value is safe to use regardless of whether the
// prefilter exists -- it is safe to call Expr() and Opts() on a nil prefilter.
func ExtractPrefilterFromQuery(queryContext table.QueryContext) (*dataflatten.Prefilter, error) {
	dataPrefilter := tablehelpers.GetConstraints(queryContext, "prefilter")
	if len(dataPrefilter) == 0 {
		return nil, nil
	}
	if len(dataPrefilter) > 1 {
		return nil, fmt.Errorf("got %d prefilter constraints, expected only 1", len(dataPrefilter))
	}
	prefilterExpr := dataPrefilter[0]
	if strings.TrimSpace(prefilterExpr) == "" {
		return nil, nil
	}
	p, err := dataflatten.NewPrefilter(prefilterExpr)
	if err != nil {
		return nil, fmt.Errorf("creating prefilter: %w", err)
	}

	return p, nil
}
