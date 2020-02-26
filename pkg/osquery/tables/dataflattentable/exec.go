package dataflattentable

import (
	"bytes"
	"context"
	"os/exec"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

func TablePluginExec(client *osquery.ExtensionManagerClient, logger log.Logger, tableName string, dataSourceType DataSourceType, execArgs []string) *table.Plugin {
	columns := []table.ColumnDefinition{
		table.TextColumn("fullkey"),
		table.TextColumn("parent"),
		table.TextColumn("key"),
		table.TextColumn("value"),
		table.TextColumn("query"),
	}

	t := &Table{
		client:    client,
		logger:    level.NewFilter(logger, level.AllowInfo()),
		tableName: tableName,
		execArgs:  execArgs,
	}

	switch dataSourceType {
	case PlistType:
		t.execDataFunc = dataflatten.Plist
	case JsonType:
		t.execDataFunc = dataflatten.Json
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
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := exec.CommandContext(ctx, t.execArgs[0], t.execArgs[1:]...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	level.Debug(t.logger).Log("msg", "calling %s", "args", t.execArgs[0], cmd.Args)

	if err := cmd.Run(); err != nil {
		return nil, errors.Wrapf(err, "calling %s. Got: %s", t.execArgs[0], string(stderr.Bytes()))
	}

	return stdout.Bytes(), nil
}

func (t *Table) getRowsFromOutput(dataQuery string, execOutput []byte) []map[string]string {
	var results []map[string]string

	flattenOpts := []dataflatten.FlattenOpts{}

	if dataQuery != "" {
		flattenOpts = append(flattenOpts, dataflatten.WithQuery(strings.Split(dataQuery, "/")))
	}

	if t.logger != nil {
		flattenOpts = append(flattenOpts, dataflatten.WithLogger(t.logger))
	}

	data, err := t.execDataFunc(execOutput, flattenOpts...)
	if err != nil {
		level.Info(t.logger).Log("msg", "failure flattening output", "err", err)
		return nil
	}

	for _, row := range data {
		p, k := row.ParentKey("/")

		res := map[string]string{
			"fullkey": row.StringPath("/"),
			"parent":  p,
			"key":     k,
			"value":   row.Value,
			"query":   dataQuery,
		}
		results = append(results, res)
	}
	return results
}
