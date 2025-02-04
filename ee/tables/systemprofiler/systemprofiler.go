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
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/ee/dataflatten"
	"github.com/kolide/launcher/ee/tables/dataflattentable"
	"github.com/kolide/launcher/ee/tables/tablehelpers"
	"github.com/kolide/launcher/ee/tables/tablewrapper"
	"github.com/kolide/launcher/pkg/traces"
	"github.com/osquery/osquery-go/plugin/table"
)

const (
	typeAllowedCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_"
	maxDataTypesPerQuery  = 3
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

func TablePlugin(flags types.Flags, slogger *slog.Logger) *table.Plugin {
	columns := dataflattentable.Columns(
		table.TextColumn("parentdatatype"),
		table.TextColumn("datatype"),
		table.TextColumn("detaillevel"),
	)

	t := &Table{
		slogger:   slogger.With("table", "kolide_system_profiler"),
		tableName: "kolide_system_profiler",
	}

	return tablewrapper.New(flags, slogger, t.tableName, columns, t.generate)
}

func (t *Table) generate(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
	ctx, span := traces.StartSpan(ctx, "table_name", "kolide_system_profiler")
	defer span.End()

	var results []map[string]string

	requestedDatatypes := tablehelpers.GetConstraints(queryContext, "datatype",
		tablehelpers.WithAllowedCharacters(typeAllowedCharacters),
		tablehelpers.WithSlogger(t.slogger),
	)

	if len(requestedDatatypes) == 0 {
		return results, fmt.Errorf("the %s table requires that you specify a constraint for datatype", t.tableName)
	}

	// Check for maximum number of datatypes
	if len(requestedDatatypes) > maxDataTypesPerQuery {
		return results, fmt.Errorf("maximum of %d datatypes allowed per query", maxDataTypesPerQuery)
	}

	// Get detaillevel constraints
	detailLevel := ""
	detailLevels := tablehelpers.GetConstraints(queryContext, "detaillevel",
		tablehelpers.WithAllowedValues(knownDetailLevels),
		tablehelpers.WithSlogger(t.slogger),
	)

	if len(detailLevels) > 0 {
		detailLevel = detailLevels[0]
	}

	if len(detailLevels) > 1 {
		t.slogger.Log(ctx, slog.LevelWarn,
			"received multiple detaillevel constraints, only using the first one",
		)
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
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

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
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

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
