//+build darwin

// Package systemprofiler provides a suite of tables for the various
// subcommands of `system_profiler`
package systemprofiler

import (
	"bytes"
	"context"
	"os/exec"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/groob/plist"
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
		logger:    logger, //level.NewFilter(logger, level.AllowInfo()),
		tableName: "kolide_system_profiler",
	}

	return table.NewPlugin(t.tableName, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	//ctx = ctxlog.NewContext(ctx, t.logger)

	var results []map[string]string

	datatypeQ, ok := queryContext.Constraints["datatype"]
	if !ok || len(datatypeQ.Constraints) == 0 {
		return results, errors.Errorf("The %s table requires that you specify a constraint for datatype", t.tableName)
	}

	datatypes := []string{}

	for _, datatypeConstraint := range datatypeQ.Constraints {
		datatype := datatypeConstraint.Expression

		// If the constraint is the magic "%", then don't add any args.
		if datatype == "%" {
			continue
		}

		datatypes = append(datatypes, datatype)
	}

	level.Debug(t.logger).Log("msg", "seph We're using args", "args", datatypes)

	flattenOpts := []dataflatten.FlattenOpts{
		//dataflatten.WithQuery(strings.Split(dataQuery, "/")),
	}

	if t.logger != nil {
		flattenOpts = append(flattenOpts, dataflatten.WithLogger(t.logger))
	}

	systemProfilerOutput, err := t.execSystemProfiler(ctx, datatypes)
	if err != nil {
		return results, errors.Wrap(err, "exec")
	}

	// Now that we have output, parse it into the underlying result structure.
	var systemProfilerResults []Result

	if err := plist.Unmarshal(systemProfilerOutput, &systemProfilerResults); err != nil {
		return results, errors.Wrap(err, "unmarshalling system profiler output")
	}

	// Now create the osquery results set from the parsed data
	for _, systemProfilerResult := range systemProfilerResults {
		dataType := systemProfilerResult.DataType

		data, err := dataflatten.Flatten(systemProfilerResult.Items)
		if err != nil {
			level.Info(t.logger).Log("msg", "failure parsing system_profile output", "err", err)
			return results, errors.Wrap(err, "parsing data")
		}

		for _, row := range data {
			p, k := row.ParentKey("/")

			res := map[string]string{
				"datatype": dataType,
				"fullkey":  row.StringPath("/"),
				"parent":   p,
				"key":      k,
				"value":    row.Value,
				//"query":   dataQuery,
			}
			results = append(results, res)
		}
	}

	//spew.Dump(results)

	return results, nil
}

type Property struct {
	Order                string `plist:"_order"`
	SuppressLocalization string `plist:"_suppressLocalization"`
	DetailLevel          string `plist:"_detailLevel"`
}

type Result struct {
	Items          []map[string]interface{} `plist:"_items"`
	DetailLevel    string                   `plist:"_detailLevel"`
	DataType       string                   `plist:"_dataType"`
	SPCommandLine  []string                 `plist:"_SPCommandLineArguments"`
	ParentDataType string                   `plist:"_parentDataType"`
	Properties     map[string]Property      `plist:"_properties"`
}

func (t *Table) execSystemProfiler(ctx context.Context, subcommands []string) ([]byte, error) {
	// logger := log.With(ctxlog.FromContext(ctx), "caller", log.DefaultCaller)

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
