package dataflattentable

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
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

func WithBinDirs(binDirs ...string) ExecTableOpt {
	return func(t *Table) {
		t.binDirs = binDirs
	}
}

func TablePluginExec(client *osquery.ExtensionManagerClient, logger log.Logger, tableName string, dataSourceType DataSourceType, execArgs []string, opts ...ExecTableOpt) *table.Plugin {
	columns := Columns()

	t := &Table{
		client:            client,
		logger:            level.NewFilter(logger, level.AllowInfo()),
		tableName:         tableName,
		execArgs:          execArgs,
		keyValueSeparator: ":",
	}

	for _, opt := range opts {
		opt(t)
	}

	switch dataSourceType {
	case PlistType:
		t.execDataFunc = dataflatten.Plist
	case JsonType:
		t.execDataFunc = dataflatten.Json
	case KeyValueType:
		// TODO: allow callers of TablePluginExec to specify the record
		// splitting strategy
		t.execDataFunc = dataflatten.StringDelimitedFunc(t.keyValueSeparator, dataflatten.DuplicateKeys)
	default:
		panic("Unknown data source type")
	}

	return table.NewPlugin(t.tableName, columns, t.generateExec)
}

func (t *Table) generateExec(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	execBytes, err := t.exec(ctx)
	if err != nil {
		return results, errors.Wrap(err, "exec")
	}

	if q, ok := queryContext.Constraints["query"]; ok && len(q.Constraints) != 0 {
		for _, constraint := range q.Constraints {
			dataQuery := constraint.Expression
			results = append(results, t.getRowsFromOutput(dataQuery, execBytes)...)
		}
	} else {
		results = append(results, t.getRowsFromOutput("", execBytes)...)
	}

	return results, nil
}

func (t *Table) exec(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 50*time.Second)
	defer cancel()

	possibleBinaries := []string{}

	if t.binDirs == nil || len(t.binDirs) == 0 {
		possibleBinaries = []string{t.execArgs[0]}
	} else {
		for _, possiblePath := range t.binDirs {
			possibleBinaries = append(possibleBinaries, filepath.Join(possiblePath, t.execArgs[0]))
		}
	}

	for _, execPath := range possibleBinaries {
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		cmd := exec.CommandContext(ctx, execPath, t.execArgs[1:]...)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		level.Debug(t.logger).Log("msg", "calling %s", "args", cmd.String())

		if err := cmd.Run(); os.IsNotExist(err) {
			// try the next binary
			continue
		} else if err != nil {
			return nil, errors.Wrapf(err, "calling %s. Got: %s", t.execArgs[0], string(stderr.Bytes()))
		}

		// success!
		return stdout.Bytes(), nil
	}
	// Shouldn't be possible to get here.
	return nil, errors.New("Impossible Error: No possible exec")
}

func (t *Table) getRowsFromOutput(dataQuery string, execOutput []byte) []map[string]string {
	flattenOpts := []dataflatten.FlattenOpts{
		dataflatten.WithLogger(t.logger),
		dataflatten.WithQuery(strings.Split(dataQuery, "/")),
	}

	data, err := t.execDataFunc(execOutput, flattenOpts...)
	if err != nil {
		level.Info(t.logger).Log("msg", "failure flattening output", "err", err)
		return nil
	}

	return ToMap(data, dataQuery, nil)
}
