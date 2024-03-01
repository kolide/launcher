package dataflattentable

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/osquery/osquery-go"
	"github.com/osquery/osquery-go/plugin/table"
)

type DataSourceType int

const (
	PlistType DataSourceType = iota + 1
	JsonType
	JsonlType
	JWTType
	ExecType
	XmlType
	IniType
	KeyValueType
	LineSepType
)

type Table struct {
	slogger   *slog.Logger
	tableName string

	flattenFileFunc  func(string, ...dataflatten.FlattenOpts) ([]dataflatten.Row, error)
	flattenBytesFunc func([]byte, ...dataflatten.FlattenOpts) ([]dataflatten.Row, error)

	cmdGen   allowedcmd.AllowedCommand
	execArgs []string

	keyValueSeparator  string
	lineFieldSeparator string
	lineHeaders        []string
	skipFirstNLines    int
}

// AllTablePlugins is a helper to return all the expected flattening tables.
func AllTablePlugins(slogger *slog.Logger) []osquery.OsqueryPlugin {
	return []osquery.OsqueryPlugin{
		TablePlugin(slogger, JsonType),
		TablePlugin(slogger, XmlType),
		TablePlugin(slogger, IniType),
		TablePlugin(slogger, PlistType),
		TablePlugin(slogger, JsonlType),
		TablePlugin(slogger, JWTType),
	}
}

func TablePlugin(slogger *slog.Logger, dataSourceType DataSourceType) osquery.OsqueryPlugin {
	columns := Columns(table.TextColumn("path"))

	t := &Table{}

	switch dataSourceType {
	case PlistType:
		t.flattenFileFunc = dataflatten.PlistFile
		t.tableName = "kolide_plist"
	case JsonType:
		t.flattenFileFunc = dataflatten.JsonFile
		t.tableName = "kolide_json"
	case JsonlType:
		t.flattenFileFunc = dataflatten.JsonlFile
		t.tableName = "kolide_jsonl"
	case XmlType:
		t.flattenFileFunc = dataflatten.XmlFile
		t.tableName = "kolide_xml"
	case IniType:
		t.flattenFileFunc = dataflatten.IniFile
		t.tableName = "kolide_ini"
	case JWTType:
		t.flattenFileFunc = dataflatten.JWTFile
		t.tableName = "kolide_jwt"
	default:
		panic("Unknown data source type")
	}

	t.slogger = slogger.With("table", t.tableName)

	return table.NewPlugin(t.tableName, columns, t.generate)

}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	requestedPaths := tablehelpers.GetConstraints(queryContext, "path")
	if len(requestedPaths) == 0 {
		return results, fmt.Errorf("The %s table requires that you specify a single constraint for path", t.tableName)
	}

	for _, requestedPath := range requestedPaths {

		// We take globs in via the sql %, but glob needs *. So convert.
		filePaths, err := filepath.Glob(strings.ReplaceAll(requestedPath, `%`, `*`))
		if err != nil {
			return results, fmt.Errorf("bad glob: %w", err)
		}

		for _, filePath := range filePaths {
			for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
				subresults, err := t.generatePath(ctx, filePath, dataQuery)
				if err != nil {
					t.slogger.Log(ctx, slog.LevelInfo,
						"failed to get data for path",
						"path", filePath,
						"err", err,
					)
					continue
				}

				results = append(results, subresults...)
			}
		}
	}
	return results, nil
}

func (t *Table) generatePath(ctx context.Context, filePath string, dataQuery string) ([]map[string]string, error) {
	flattenOpts := []dataflatten.FlattenOpts{
		dataflatten.WithSlogger(t.slogger),
		dataflatten.WithNestedPlist(),
		dataflatten.WithQuery(strings.Split(dataQuery, "/")),
	}

	data, err := t.flattenFileFunc(filePath, flattenOpts...)
	if err != nil {
		t.slogger.Log(ctx, slog.LevelInfo,
			"failure parsing file",
			"file", filePath,
		)
		return nil, fmt.Errorf("parsing data: %w", err)
	}

	rowData := map[string]string{
		"path": filePath,
	}

	return ToMap(data, dataQuery, rowData), nil
}
