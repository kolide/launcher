package dataflattentable

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/observability"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/osquery/osquery-go"
	"github.com/osquery/osquery-go/plugin/table"
)

type DataSourceType struct {
	tableName   string
	description string

	// Required: factory returning the bytes flatten func. Receives QueryContext
	// so types can vary behavior per-query (e.g., protobuf schema selection).
	flattenBytesFunc func(table.QueryContext) dataflatten.DataFunc

	// Optional: factory returning the file flatten func. When nil, the table
	// auto-generates one that reads the file with os.ReadFile and delegates
	// to flattenBytesFunc. Only needed when file handling differs from
	// "read bytes, parse bytes" (e.g., JSON's UTF-16 fallback, XML's reader API).
	flattenFileFunc func(table.QueryContext) dataflatten.DataFileFunc

	extraColumns []table.ColumnDefinition
}

var allTypes = []DataSourceType{
	{
		tableName:        "kolide_json",
		description:      "Parses JSON files or raw JSON data and returns flattened key-value pairs. Requires a WHERE path = or raw_data = constraint. Supports a query constraint for filtering specific keys. Useful for reading any JSON configuration or data file.",
		flattenBytesFunc: func(_ table.QueryContext) dataflatten.DataFunc { return dataflatten.Json },
		flattenFileFunc:  func(_ table.QueryContext) dataflatten.DataFileFunc { return dataflatten.JsonFile },
	},
	{
		tableName:        "kolide_xml",
		description:      "Parses XML files or raw XML data and returns flattened key-value pairs. Requires a WHERE path = or raw_data = constraint. Supports a query constraint for filtering specific keys. Useful for reading XML configuration or data files.",
		flattenBytesFunc: func(_ table.QueryContext) dataflatten.DataFunc { return dataflatten.Xml },
		flattenFileFunc:  func(_ table.QueryContext) dataflatten.DataFileFunc { return dataflatten.XmlFile },
	},
	{
		tableName:        "kolide_ini",
		description:      "Parses INI files or raw INI data and returns flattened key-value pairs. Requires a WHERE path = or raw_data = constraint. Supports a query constraint for filtering specific keys. Useful for reading INI-style configuration files.",
		flattenBytesFunc: func(_ table.QueryContext) dataflatten.DataFunc { return dataflatten.Ini },
		flattenFileFunc:  func(_ table.QueryContext) dataflatten.DataFileFunc { return dataflatten.IniFile },
	},
	{
		tableName:        "kolide_plist",
		description:      "Parses Apple plist files or raw plist data and returns flattened key-value pairs. Requires a WHERE path = or raw_data = constraint. Supports a query constraint for filtering specific keys. Useful for reading macOS preference files, application plists, and system configuration.",
		flattenBytesFunc: func(_ table.QueryContext) dataflatten.DataFunc { return dataflatten.Plist },
	},
	{
		tableName:        "kolide_jsonl",
		description:      "Parses JSONL (JSON Lines) files or raw data and returns flattened key-value pairs. Requires a WHERE path = or raw_data = constraint. Supports a query constraint for filtering specific keys. Useful for reading line-delimited JSON log files.",
		flattenBytesFunc: func(_ table.QueryContext) dataflatten.DataFunc { return dataflatten.Jsonl },
	},
	{
		tableName:        "kolide_protobuf",
		description:      "Parses marshaled protobuf files or raw protobuf data and returns flattened key-value pairs. Field numbers are used as keys since protobuf wire format is schema-less. Requires a WHERE path = or raw_data = constraint.",
		flattenBytesFunc: func(_ table.QueryContext) dataflatten.DataFunc { return dataflatten.Protobuf },
	},
}

type Table struct {
	slogger   *slog.Logger
	tableName string

	flattenFileFunc  dataflatten.DataFileFunc
	flattenBytesFunc dataflatten.DataFunc
}

// AllTablePlugins is a helper to return all the expected flattening tables.
func AllTablePlugins(flags types.Flags, slogger *slog.Logger) []osquery.OsqueryPlugin {
	plugins := make([]osquery.OsqueryPlugin, 0, len(allTypes))
	for _, dst := range allTypes {
		plugins = append(plugins, TablePlugin(flags, slogger, dst))
	}
	return plugins
}

func TablePlugin(flags types.Flags, slogger *slog.Logger, dst DataSourceType) osquery.OsqueryPlugin {
	columns := Columns(append(
		[]table.ColumnDefinition{table.TextColumn("path"), table.TextColumn("raw_data")},
		dst.extraColumns...,
	)...)

	tableSlogger := slogger.With("table", dst.tableName)

	generate := func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		bytesFn := dst.flattenBytesFunc(queryContext)

		var fileFn dataflatten.DataFileFunc
		if dst.flattenFileFunc != nil {
			fileFn = dst.flattenFileFunc(queryContext)
		} else {
			fileFn = func(file string, opts ...dataflatten.FlattenOpts) ([]dataflatten.Row, error) {
				raw, err := os.ReadFile(file)
				if err != nil {
					return nil, fmt.Errorf("reading %s prior to flattening: %w", file, err)
				}
				return bytesFn(raw, opts...)
			}
		}

		t := &Table{
			slogger:          tableSlogger,
			tableName:        dst.tableName,
			flattenBytesFunc: bytesFn,
			flattenFileFunc:  fileFn,
		}
		return t.generate(ctx, queryContext)
	}

	var opts []tablewrapper.TablePluginOption
	if dst.description != "" {
		opts = append(opts, tablewrapper.WithDescription(dst.description))
	}
	opts = append(opts, tablewrapper.WithNote(EAVNote))

	return tablewrapper.New(flags, slogger, dst.tableName, columns, generate, opts...)
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
