package dataflattentable

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go"
	"github.com/osquery/osquery-go/plugin/table"
)

type DataSourceType struct {
	flattenBytesFunc func(string) dataflatten.DataFunc
	flattenFileFunc  func(string) dataflatten.DataFileFunc
	tableName        string
}

var (
	PlistType = DataSourceType{
		flattenBytesFunc: func(_ string) dataflatten.DataFunc { return dataflatten.Plist },
		flattenFileFunc:  func(_ string) dataflatten.DataFileFunc { return dataflatten.PlistFile },
		tableName:        "kolide_plist",
	}
	JsonType = DataSourceType{
		flattenBytesFunc: func(_ string) dataflatten.DataFunc { return dataflatten.Json },
		flattenFileFunc:  func(_ string) dataflatten.DataFileFunc { return dataflatten.JsonFile },
		tableName:        "kolide_json",
	}
	JsonlType = DataSourceType{
		flattenBytesFunc: func(_ string) dataflatten.DataFunc { return dataflatten.Jsonl },
		flattenFileFunc:  func(_ string) dataflatten.DataFileFunc { return dataflatten.JsonlFile },
		tableName:        "kolide_jsonl",
	}
	XmlType = DataSourceType{
		flattenBytesFunc: func(_ string) dataflatten.DataFunc { return dataflatten.Xml },
		flattenFileFunc:  func(_ string) dataflatten.DataFileFunc { return dataflatten.XmlFile },
		tableName:        "kolide_xml",
	}
	IniType = DataSourceType{
		flattenBytesFunc: func(_ string) dataflatten.DataFunc { return dataflatten.Ini },
		flattenFileFunc:  func(_ string) dataflatten.DataFileFunc { return dataflatten.IniFile },
		tableName:        "kolide_ini",
	}
	KeyValueType = DataSourceType{
		flattenBytesFunc: func(kvDelimiter string) dataflatten.DataFunc {
			return dataflatten.StringDelimitedFunc(kvDelimiter, dataflatten.DuplicateKeys)
		},
		flattenFileFunc: func(kvDelimiter string) dataflatten.DataFileFunc {
			// This func is unused -- only flattenBytesFunc is used, in `TablePluginExec` --
			// but we include it here for completeness.
			return func(file string, opts ...dataflatten.FlattenOpts) ([]dataflatten.Row, error) {
				f, err := os.Open(file)
				if err != nil {
					return nil, fmt.Errorf("unable to access file: %w", err)
				}

				defer f.Close()

				rawdata, err := io.ReadAll(f)
				if err != nil {
					return nil, fmt.Errorf("unable to read JWT: %w", err)
				}

				flattenFunc := dataflatten.StringDelimitedFunc(kvDelimiter, dataflatten.DuplicateKeys)

				return flattenFunc(rawdata, opts...)
			}
		},
		tableName: "", // table name not set for key-value type as there are multiple tables constructed via `TablePluginExec` using this type
	}
)

func (d DataSourceType) FlattenBytesFunc(kvDelimiter string) dataflatten.DataFunc {
	return d.flattenBytesFunc(kvDelimiter)
}

func (d DataSourceType) FlattenFileFunc(kvDelimiter string) dataflatten.DataFileFunc {
	return d.flattenFileFunc(kvDelimiter)
}

func (d DataSourceType) TableName() string {
	return d.tableName
}

type Table struct {
	slogger   *slog.Logger
	tableName string

	flattenFileFunc  dataflatten.DataFileFunc
	flattenBytesFunc dataflatten.DataFunc

	cmdGen   allowedcmd.AllowedCommand
	execArgs []string

	keyValueSeparator string
}

// AllTablePlugins is a helper to return all the expected flattening tables.
func AllTablePlugins(flags types.Flags, slogger *slog.Logger) []osquery.OsqueryPlugin {
	return []osquery.OsqueryPlugin{
		TablePlugin(flags, slogger, JsonType),
		TablePlugin(flags, slogger, XmlType),
		TablePlugin(flags, slogger, IniType),
		TablePlugin(flags, slogger, PlistType),
		TablePlugin(flags, slogger, JsonlType),
	}
}

func TablePlugin(flags types.Flags, slogger *slog.Logger, dataSourceType DataSourceType) osquery.OsqueryPlugin {
	columns := Columns(table.TextColumn("path"), table.TextColumn("raw_data"))

	t := &Table{
		tableName:        dataSourceType.TableName(),
		flattenFileFunc:  dataSourceType.FlattenFileFunc(""),
		flattenBytesFunc: dataSourceType.FlattenBytesFunc(""),
	}

	t.slogger = slogger.With("table", t.tableName)

	return tablewrapper.New(flags, slogger, t.tableName, columns, t.generate)

}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "table_name", t.tableName)
	defer span.End()

	var results []map[string]string

	requestedPaths := tablehelpers.GetConstraints(queryContext, "path")
	requestedRawDatas := tablehelpers.GetConstraints(queryContext, "raw_data")

	if len(requestedPaths) == 0 && len(requestedRawDatas) == 0 {
		return results, fmt.Errorf("The %s table requires that you specify at least one of 'path' or 'raw_data'", t.tableName)
	}

	flattenOpts := []dataflatten.FlattenOpts{
		dataflatten.WithSlogger(t.slogger),
		dataflatten.WithNestedPlist(),
	}

	for _, requestedPath := range requestedPaths {

		// We take globs in via the sql %, but glob needs *. So convert.
		filePaths, err := filepath.Glob(strings.ReplaceAll(requestedPath, `%`, `*`))
		if err != nil {
			return results, fmt.Errorf("bad glob: %w", err)
		}

		for _, filePath := range filePaths {
			for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
				subresults, err := t.generatePath(ctx, filePath, dataQuery, append(flattenOpts, dataflatten.WithQuery(strings.Split(dataQuery, "/")))...)
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

	for _, rawdata := range requestedRawDatas {
		for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
			subresults, err := t.generateRawData(ctx, rawdata, dataQuery, append(flattenOpts, dataflatten.WithQuery(strings.Split(dataQuery, "/")))...)
			if err != nil {
				t.slogger.Log(ctx, slog.LevelInfo,
					"failed to generate for raw_data",
					"err", err,
				)
				continue
			}

			results = append(results, subresults...)
		}
	}

	return results, nil
}

func (t *Table) generateRawData(ctx context.Context, rawdata string, dataQuery string, flattenOpts ...dataflatten.FlattenOpts) ([]map[string]string, error) {
	data, err := t.flattenBytesFunc([]byte(rawdata), flattenOpts...)
	if err != nil {
		t.slogger.Log(ctx, slog.LevelInfo,
			"failure parsing raw data",
			"err", err,
		)
		return nil, fmt.Errorf("parsing data: %w", err)
	}

	rowData := map[string]string{
		"raw_data": rawdata,
	}

	return ToMap(data, dataQuery, rowData), nil
}

func (t *Table) generatePath(ctx context.Context, filePath string, dataQuery string, flattenOpts ...dataflatten.FlattenOpts) ([]map[string]string, error) {
	ctx, span := observability.StartSpan(ctx, "path", filePath)
	defer span.End()

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
