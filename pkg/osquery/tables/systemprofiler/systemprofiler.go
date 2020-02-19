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
	"github.com/groob/plist"
	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/osquery-go"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/pkg/errors"
)

const systemprofilerPath = "/usr/sbin/system_profiler"

var knownDetailLevels = []string{
	"mini",  // short report (contains no identifying or personal information)
	"basic", // basic hardware and network information
	"full",  // all available information
}

type Property struct {
	Order                string `plist:"_order"`
	SuppressLocalization string `plist:"_suppressLocalization"`
	DetailLevel          string `plist:"_detailLevel"`
}

type Result struct {
	Items          []interface{} `plist:"_items"`
	DataType       string        `plist:"_dataType"`
	SPCommandLine  []string      `plist:"_SPCommandLineArguments"`
	ParentDataType string        `plist:"_parentDataType"`

	// These would be nice to add, but they come back with inconsistent
	// types, so doing a straight unmarshal is hard.
	// DetailLevel    int                 `plist:"_detailLevel"`
	// Properties     map[string]Property `plist:"_properties"`
}

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
		table.TextColumn("parentdatatype"),

		table.TextColumn("query"),
		table.TextColumn("datatype"),
		table.TextColumn("detaillevel"),
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

	requestedDatatypes := []string{}

	datatypeQ, ok := queryContext.Constraints["datatype"]
	if !ok || len(datatypeQ.Constraints) == 0 {
		return results, errors.Errorf("The %s table requires that you specify a constraint for datatype", t.tableName)
	}

	for _, datatypeConstraint := range datatypeQ.Constraints {
		dt := datatypeConstraint.Expression

		// If the constraint is the magic "%", it's eqivlent to an `all` style
		if dt == "%" {
			requestedDatatypes = []string{}
			break
		}

		requestedDatatypes = append(requestedDatatypes, dt)
	}

	var detailLevel string
	if q, ok := queryContext.Constraints["detaillevel"]; ok && len(q.Constraints) != 0 {
		if len(q.Constraints) > 1 {
			level.Info(t.logger).Log("msg", "WARNING: Only using the first detaillevel request")
		}

		dl := q.Constraints[0].Expression
		for _, known := range knownDetailLevels {
			if known == dl {
				detailLevel = dl
			}
		}

	}

	systemProfilerOutput, err := t.execSystemProfiler(ctx, detailLevel, requestedDatatypes)
	if err != nil {
		return results, errors.Wrap(err, "exec")
	}

	if q, ok := queryContext.Constraints["query"]; ok && len(q.Constraints) != 0 {
		for _, constraint := range q.Constraints {
			dataQuery := constraint.Expression
			results = append(results, t.getRowsFromOutput(dataQuery, detailLevel, systemProfilerOutput)...)
		}
	} else {
		results = append(results, t.getRowsFromOutput("", detailLevel, systemProfilerOutput)...)
	}

	return results, nil
}

func (t *Table) getRowsFromOutput(dataQuery, detailLevel string, systemProfilerOutput []byte) []map[string]string {
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
	//data, err := dataflatten.Plist(systemProfilerOutput, flattenOpts...)
	var systemProfilerResults []Result
	if err := plist.Unmarshal(systemProfilerOutput, &systemProfilerResults); err != nil {
		level.Info(t.logger).Log("msg", "error unmarshalling system_profile output", "err", err)
		return nil
	}

	for _, systemProfilerResult := range systemProfilerResults {

		dataType := systemProfilerResult.DataType

		data, err := dataflatten.Flatten(systemProfilerResult.Items, flattenOpts...)

		if err != nil {
			level.Info(t.logger).Log("msg", "failure flattening system_profile output", "err", err)
			return nil
		}

		for _, row := range data {
			p, k := row.ParentKey("/")

			res := map[string]string{
				"datatype":       dataType,
				"parentdatatype": systemProfilerResult.ParentDataType,
				"fullkey":        row.StringPath("/"),
				"parent":         p,
				"key":            k,
				"value":          row.Value,
				"query":          dataQuery,
				"detaillevel":    detailLevel,
			}
			results = append(results, res)
		}
	}

	return results
}

func (t *Table) execSystemProfiler(ctx context.Context, detailLevel string, subcommands []string) ([]byte, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	args := []string{"-xml"}

	if detailLevel != "" {
		args = append(args, "-detailLevel", detailLevel)
	}

	args = append(args, subcommands...)

	cmd := exec.CommandContext(ctx, systemprofilerPath, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	level.Debug(t.logger).Log("msg", "calling system_profiler", "args", cmd.Args)

	if err := cmd.Run(); err != nil {
		return nil, errors.Wrapf(err, "calling system_profiler. Got: %s", string(stderr.Bytes()))
	}

	return stdout.Bytes(), nil
}
