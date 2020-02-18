//+build darwin

// Package systemprofiler provides a suite of tables for the various
// subcommands of `system_profiler`
package systemprofiler

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

const systemprofilerPath = "/usr/sbin/system_profiler"

type Table struct {
	client    *osquery.ExtensionManagerClient
	logger    log.Logger
	tableName string
}

func TablePlugin(client *osquery.ExtensionManagerClient, logger log.Logger) *table.Plugin {

	columns := []table.ColumnDefinition{
		table.TextColumn("fullkey"),
		table.TextColumn("parent"),
		table.TextColumn("key"),
		table.TextColumn("value"),
		table.TextColumn("query"),
		table.TextColumn("datatype"),
	}

	t := &Table{
		client:    client,
		logger:    level.NewFilter(logger, level.AllowInfo()),
		tableName: "kolide_system_profiler",
	}

	return table.NewPlugin(t.tableName, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	datatypeQ, ok := queryContext.Constraints["datatype"]
	if !ok || len(datatypeQ.Constraints) == 0 {
		return results, errors.Errorf("The %s table requires that you specify a constraint for datatype", t.tableName)
	}

	// For each requested datatype, run system profiler This
	// implementation has a couple of limitations -- It's an invocation
	// per dataType requested, and it does not support an `all` type.
	for _, datatypeConstraint := range datatypeQ.Constraints {
		dataType := datatypeConstraint.Expression

		systemProfilerOutput, err := t.execSystemProfiler(ctx, []string{dataType})
		if err != nil {
			return results, errors.Wrap(err, "exec")
		}

		if q, ok := queryContext.Constraints["query"]; ok && len(q.Constraints) != 0 {
			for _, constraint := range q.Constraints {
				dataQuery := constraint.Expression
				results = append(results, t.getRowsFromOutput(dataType, dataQuery, systemProfilerOutput)...)
			}
		} else {
			results = append(results, t.getRowsFromOutput(dataType, "", systemProfilerOutput)...)
		}
	}

	return results, nil
}

func (t *Table) getRowsFromOutput(dataType string, dataQuery string, systemProfilerOutput []byte) []map[string]string {
	var results []map[string]string

	flattenOpts := []dataflatten.FlattenOpts{}

	if dataQuery != "" {
		flattenOpts = append(flattenOpts, dataflatten.WithQuery(strings.Split(dataQuery, "/")))
	}

	if t.logger != nil {
		flattenOpts = append(flattenOpts, dataflatten.WithLogger(t.logger))
	}

	// Now that we have output, parse it into the underlying result
	// structure. It might be nice to pre-process this, and remove the
	// _properties, but that's hard to do cleanly, so just flatten it
	// directly.
	data, err := dataflatten.Plist(systemProfilerOutput, flattenOpts...)
	if err != nil {
		level.Info(t.logger).Log("msg", "failure flattening system_profile output", "err", err)
		return nil
	}

	for _, row := range data {
		p, k := row.ParentKey("/")

		res := map[string]string{
			"datatype": dataType,
			"fullkey":  row.StringPath("/"),
			"parent":   p,
			"key":      k,
			"value":    row.Value,
			"query":    dataQuery,
		}
		results = append(results, res)
	}

	return results
}

func (t *Table) execSystemProfiler(ctx context.Context, subcommands []string) ([]byte, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	subcommands = append(subcommands, "-xml")

	cmd := exec.CommandContext(ctx, systemprofilerPath, subcommands...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	level.Debug(t.logger).Log("msg", "calling system_profiler", "args", cmd.Args)

	if err := cmd.Run(); err != nil {
		return nil, errors.Wrapf(err, "calling system_profiler. Got: %s", string(stderr.Bytes()))
	}

	return stdout.Bytes(), nil
}
