package dataflattentable

import (
	"context"
	"log/slog"

	"os"
	"strings"

	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

type ExecTableOpt func(*Table)

// WithKVSeparator sets the delimiter between key and value. It replaces the
// default ":" in dataflattentable.Table
func WithKVSeparator(separator string) ExecTableOpt {
	return func(t *Table) {
		t.keyValueSeparator = separator
	}
}

// WithLineSeparator sets the delimiter between fields on a line of data.
// default "" in dataflattentable.Table (assumes whitespace separator in lines of data)
func WithLineSeparator(separator string) ExecTableOpt {
	return func(t *Table) {
		t.lineValueSeparator = separator
	}
}

// WithLineHeaders sets the slice of headers that will be used for fields in each line of data.
// default []string{} in dataflattentable.Table (sets headers from first available line of data)
func WithLineHeaders(headers []string) ExecTableOpt {
	return func(t *Table) {
		t.lineHeaders = headers
	}
}

// SkipFirstNLines sets the number of initial lines to skip over before reading from lines of data.
// default 0 in dataflattentable.Table
func SkipFirstNLines(skip int) ExecTableOpt {
	return func(t *Table) {
		t.skipFirstNLines = skip
	}
}

func TablePluginExec(slogger *slog.Logger, tableName string, dataSourceType DataSourceType, cmdGen allowedcmd.AllowedCommand, execArgs []string, opts ...ExecTableOpt) *table.Plugin {
	columns := Columns()

	t := &Table{
		slogger:            slogger.With("table", tableName),
		tableName:          tableName,
		cmdGen:             cmdGen,
		execArgs:           execArgs,
		keyValueSeparator:  ":",
		lineValueSeparator: "",
		lineHeaders:        []string{},
		skipFirstNLines:    0,
	}

	for _, opt := range opts {
		opt(t)
	}

	switch dataSourceType {
	case PlistType:
		t.flattenBytesFunc = dataflatten.Plist
	case JsonType:
		t.flattenBytesFunc = dataflatten.Json
	case XmlType:
		t.flattenBytesFunc = dataflatten.Xml
	case KeyValueType:
		// TODO: allow callers of TablePluginExec to specify the record
		// splitting strategy
		t.flattenBytesFunc = dataflatten.StringDelimitedFunc(t.keyValueSeparator, dataflatten.DuplicateKeys)
	case LineSepType:
		t.flattenBytesFunc = dataflatten.StringDelimitedLineFunc(t.lineValueSeparator, t.lineHeaders, t.skipFirstNLines)
	default:
		panic("Unknown data source type")
	}

	return table.NewPlugin(t.tableName, columns, t.generateExec)
}

func (t *Table) generateExec(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	execBytes, err := tablehelpers.Exec(ctx, t.slogger, 50, t.cmdGen, t.execArgs, false)
	if err != nil {
		// exec will error if there's no binary, so we never want to record that
		if os.IsNotExist(errors.Cause(err)) {
			return nil, nil
		}

		// If the exec failed for some reason, it's probably better to return no results, and log the,
		// error. Returning an error here will cause a table failure, and thus break joins
		t.slogger.Log(ctx, slog.LevelInfo,
			"failed to exec",
			"err", err,
		)
		return nil, nil
	}

	for _, dataQuery := range tablehelpers.GetConstraints(queryContext, "query", tablehelpers.WithDefaults("*")) {
		flattenOpts := []dataflatten.FlattenOpts{
			dataflatten.WithSlogger(t.slogger),
			dataflatten.WithQuery(strings.Split(dataQuery, "/")),
		}

		flattened, err := t.flattenBytesFunc(execBytes, flattenOpts...)
		if err != nil {
			t.slogger.Log(ctx, slog.LevelInfo,
				"failure flattening output",
				"err", err,
			)
			continue
		}

		results = append(results, ToMap(flattened, dataQuery, nil)...)
	}

	return results, nil
}
