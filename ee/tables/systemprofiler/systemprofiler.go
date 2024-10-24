//go:build darwin
// +build darwin

// Package systemprofiler provides a suite table wrapper around
// `system_profiler` macOS command. It supports some basic arguments
// like `detaillevel` and requested data types.
//
// Note that some detail levels and data types will have performance
// impact if requested.
//
// As the returned data is a complex nested plist, this uses the
// dataflatten tooling. (See
// https://godoc.org/github.com/kolide/launcher/ee/dataflatten)
//
// Everything, minimal details:
//
//	osquery> select count(*) from kolide_system_profiler where datatype like "%" and detaillevel = "mini";
//	+----------+
//	| count(*) |
//	+----------+
//	| 1270     |
//	+----------+
//
// Multiple data types (slightly redacted):
//
//	osquery> select fullkey, key, value, datatype from kolide_system_profiler where datatype in ("SPCameraDataType", "SPiBridgeDataType");
//	+----------------------+--------------------+------------------------------------------+-------------------+
//	| fullkey              | key                | value                                    | datatype          |
//	+----------------------+--------------------+------------------------------------------+-------------------+
//	| 0/spcamera_unique-id | spcamera_unique-id | 0x1111111111111111                       | SPCameraDataType  |
//	| 0/_name              | _name              | FaceTime HD Camera                       | SPCameraDataType  |
//	| 0/spcamera_model-id  | spcamera_model-id  | UVC Camera VendorID_1452 ProductID_30000 | SPCameraDataType  |
//	| 0/_name              | _name              | Controller Information                   | SPiBridgeDataType |
//	| 0/ibridge_build      | ibridge_build      | 14Y000                                   | SPiBridgeDataType |
//	| 0/ibridge_model_name | ibridge_model_name | Apple T1 Security Chip                   | SPiBridgeDataType |
//	+----------------------+--------------------+------------------------------------------+-------------------+
package systemprofiler

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/groob/plist"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/osquery/osquery-go/plugin/table"
)

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
	slogger   *slog.Logger
	tableName string
}

func TablePlugin(slogger *slog.Logger) *table.Plugin {
	columns := dataflattentable.Columns(
		table.TextColumn("parentdatatype"),
		table.TextColumn("datatype"),
		table.TextColumn("detaillevel"),
	)

	t := &Table{
		slogger:   slogger.With("table", "kolide_system_profiler"),
		tableName: "kolide_system_profiler",
	}

	return table.NewPlugin(t.tableName, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	var results []map[string]string

	requestedDatatypes := []string{}

	datatypeQ, ok := queryContext.Constraints["datatype"]
	if !ok || len(datatypeQ.Constraints) == 0 {
		return results, fmt.Errorf("the %s table requires that you specify a constraint for datatype", t.tableName)
	}

	// Check for maximum number of datatypes
	if len(datatypeQ.Constraints) > 3 {
		return results, fmt.Errorf("maximum of 3 datatypes allowed per query, got %d", len(datatypeQ.Constraints))
	}

	for _, datatypeConstraint := range datatypeQ.Constraints {
		dt := datatypeConstraint.Expression

		// Block the % wildcard
		if dt == "%" {
			return results, fmt.Errorf("wildcard %% is not allowed as a datatype constraint")
		}

		requestedDatatypes = append(requestedDatatypes, dt)
	}

	var detailLevel string
	if q, ok := queryContext.Constraints["detaillevel"]; ok && len(q.Constraints) != 0 {
		if len(q.Constraints) > 1 {
			t.slogger.Log(ctx, slog.LevelWarn,
				"received multiple detaillevel constraints, only using the first one",
			)
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
		return results, fmt.Errorf("exec: %w", err)
	}

	if q, ok := queryContext.Constraints["query"]; ok && len(q.Constraints) != 0 {
		for _, constraint := range q.Constraints {
			dataQuery := constraint.Expression
			results = append(results, t.getRowsFromOutput(ctx, dataQuery, detailLevel, systemProfilerOutput)...)
		}
	} else {
		results = append(results, t.getRowsFromOutput(ctx, "", detailLevel, systemProfilerOutput)...)
	}

	return results, nil
}

func (t *Table) getRowsFromOutput(ctx context.Context, dataQuery, detailLevel string, systemProfilerOutput []byte) []map[string]string {
	var results []map[string]string

	flattenOpts := []dataflatten.FlattenOpts{
		dataflatten.WithSlogger(t.slogger),
		dataflatten.WithQuery(strings.Split(dataQuery, "/")),
	}

	var systemProfilerResults []Result
	if err := plist.Unmarshal(systemProfilerOutput, &systemProfilerResults); err != nil {
		t.slogger.Log(ctx, slog.LevelInfo,
			"error unmarshalling system_profile output",
			"err", err,
		)
		return nil
	}

	for _, systemProfilerResult := range systemProfilerResults {

		dataType := systemProfilerResult.DataType

		flatData, err := dataflatten.Flatten(systemProfilerResult.Items, flattenOpts...)
		if err != nil {
			t.slogger.Log(ctx, slog.LevelInfo,
				"failure flattening system_profile output",
				"err", err,
			)
			continue
		}

		rowData := map[string]string{
			"datatype":       dataType,
			"parentdatatype": systemProfilerResult.ParentDataType,
			"detaillevel":    detailLevel,
		}

		results = append(results, dataflattentable.ToMap(flatData, dataQuery, rowData)...)
	}

	return results
}

func (t *Table) execSystemProfiler(ctx context.Context, detailLevel string, subcommands []string) ([]byte, error) {
	timeoutSeconds := 45
	if detailLevel == "full" {
		timeoutSeconds = 5 * 60
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	args := []string{"-xml"}

	if detailLevel != "" {
		args = append(args, "-detailLevel", detailLevel)
	}

	args = append(args, subcommands...)

	t.slogger.Log(ctx, slog.LevelDebug,
		"calling system_profiler",
		"args", args,
	)

	if err := tablehelpers.Run(ctx, t.slogger, timeoutSeconds, allowedcmd.SystemProfiler, args, &stdout, &stderr); err != nil {
		return nil, fmt.Errorf("calling system_profiler. Got: %s: %w", stderr.String(), err)
	}

	return stdout.Bytes(), nil
}
