package dataflattentable

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

type DataSourceType int

const (
	PlistType DataSourceType = iota + 1
	JsonType
	ExecType
	XmlType
	IniType
	KeyValueType
)

type Table struct {
	client    *osquery.ExtensionManagerClient
	logger    log.Logger
	tableName string

	dataFunc func(string, ...dataflatten.FlattenOpts) ([]dataflatten.Row, error)

	execDataFunc func([]byte, ...dataflatten.FlattenOpts) ([]dataflatten.Row, error)
	execArgs     []string
	binDirs      []string

	keyValueSeparator string
}

func TablePlugin(client *osquery.ExtensionManagerClient, logger log.Logger, dataSourceType DataSourceType) *table.Plugin {

	columns := Columns(table.TextColumn("path"))

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
	case XmlType:
		t.dataFunc = dataflatten.XmlFile
		t.tableName = "kolide_xml"
	case IniType:
		t.dataFunc = dataflatten.IniFile
		t.tableName = "kolide_ini"
	default:
		panic("Unknown data source type")
	}

	return table.NewPlugin(t.tableName, columns, t.generate)

}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	requestedPaths := tablehelpers.GetConstraints(queryContext, "path")
	if len(requestedPaths) == 0 {
		return results, errors.Errorf("The %s table requires that you specify a single constraint for path", t.tableName)
	}

	for _, requestedPath := range requestedPaths {

		// We take globs in via the sql %, but glob needs *. So convert.
		filePaths, err := filepath.Glob(strings.ReplaceAll(requestedPath, `%`, `*`))
		if err != nil {
			return results, errors.Wrap(err, "bad glob")
		}

		for _, filePath := range filePaths {
			for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
				subresults, err := t.generatePath(filePath, dataQuery)
				if err != nil {
					return results, errors.Wrapf(err, "generating for path %s with query", filePath)
				}

				results = append(results, subresults...)
			}
		}
	}
	return results, nil
}

func (t *Table) generatePath(filePath string, dataQuery string) ([]map[string]string, error) {
	flattenOpts := []dataflatten.FlattenOpts{
		dataflatten.WithLogger(t.logger),
		dataflatten.WithNestedPlist(),
		dataflatten.WithQuery(strings.Split(dataQuery, "/")),
	}

	data, err := t.dataFunc(filePath, flattenOpts...)
	if err != nil {
		level.Info(t.logger).Log("msg", "failure parsing file", "file", filePath)
		return nil, errors.Wrap(err, "parsing data")
	}

	rowData := map[string]string{
		"path": filePath,
	}

	return ToMap(data, dataQuery, rowData), nil
}
