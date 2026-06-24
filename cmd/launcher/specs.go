package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"runtime"
	"slices"
	"strings"

	"github.com/kolide/launcher/v2/ee/agent/flags"
	"github.com/kolide/launcher/v2/ee/agent/knapsack"
	"github.com/kolide/launcher/v2/pkg/launcher"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/kolide/launcher/v2/pkg/osquery/table"
	"github.com/peterbourgon/ff/v3"

	osquery "github.com/osquery/osquery-go"
	osquerytable "github.com/osquery/osquery-go/plugin/table"
)

// requiredFields accumulates -required flag values (can be specified multiple times).
type requiredFields []string

func (r *requiredFields) String() string { return fmt.Sprintf("%v", *r) }

func (r *requiredFields) Set(value string) error {
	*r = append(*r, value)
	return nil
}

// specRequiredFieldValidators maps -required flag values to functions that return true
// when the field is present and non-empty.
type requiredFieldValidator func(osquerytable.OsqueryTableSpec) bool

var specRequiredFieldValidators = map[string]func(osquerytable.OsqueryTableSpec) bool{
	"name":        func(s osquerytable.OsqueryTableSpec) bool { return len(s.Name) > 0 },
	"description": func(s osquerytable.OsqueryTableSpec) bool { return len(s.Description) > 0 },
	"url":         func(s osquerytable.OsqueryTableSpec) bool { return len(s.Url) > 0 },
	"notes":       func(s osquerytable.OsqueryTableSpec) bool { return len(s.Notes) > 0 },
	"columns":     func(s osquerytable.OsqueryTableSpec) bool { return len(s.Columns) > 0 },
	"examples":    func(s osquerytable.OsqueryTableSpec) bool { return len(s.Examples) > 0 },
}

func runSpecs(systemMultiSlogger *multislogger.MultiSlogger, args []string) error {
	flagset := flag.NewFlagSet("launcher specs", flag.ExitOnError)
	flDebug := flagset.Bool("debug", false, "enable debug logging")
	flQuiet := flagset.Bool("quiet", false, "don't print specs. Used in testing")
	flOutput := flagset.String("output", "", "write specs to file (default: stdout)")
	flMissingOk := flagset.Bool("missing-ok", false, "do not exit with error when required fields are missing or blank")
	flMerge := flagset.Bool("merge", false, "merge mode: combine the NDJSON spec files given as arguments into a single, platform-unioned JSON array")
	var flRequired requiredFields
	flagset.Var(&flRequired, "required", "field name that must be present in the spec (repeatable); warns if missing")

	if err := ff.Parse(flagset, args, ff.WithEnvVarNoPrefix()); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	if *flMerge {
		return runMergeSpecs(flagset.Args(), *flOutput)
	}

	// Reject unknown -required field names so the user gets a clear error.
	requiredFieldValidators := make(map[string]requiredFieldValidator)
	for _, field := range flRequired {
		if validator, ok := specRequiredFieldValidators[field]; !ok {
			return fmt.Errorf("unknown field to require: %s. Must be %v", field, slices.Collect(maps.Keys(specRequiredFieldValidators)))
		} else {
			requiredFieldValidators[field] = validator
		}
	}

	slogLevel := slog.LevelInfo
	if *flDebug {
		slogLevel = slog.LevelDebug
	}

	// On Windows, use stdout for log output because stderr is not available.
	// See details in https://github.com/kolide/launcher/pull/2541
	logOut := os.Stderr
	if runtime.GOOS == "windows" {
		logOut = os.Stdout
	}
	systemMultiSlogger.AddHandler(slog.NewTextHandler(logOut, &slog.HandlerOptions{
		Level:     slogLevel,
		AddSource: true,
	}))

	flagController := flags.NewFlagController(systemMultiSlogger.Logger, nil, flags.WithCmdLineOpts(&launcher.Options{}))
	k := knapsack.New(nil, flagController, nil, nil, nil)
	slogger := systemMultiSlogger.With("subprocess", "specs")

	launcherTables := table.LauncherTables(k, slogger)
	platformTables := table.PlatformTables(k, "", slogger, "")

	plugins := make([]osquery.OsqueryPlugin, 0, len(launcherTables)+len(platformTables))
	plugins = append(plugins, launcherTables...)
	plugins = append(plugins, platformTables...)

	var out io.Writer = os.Stdout
	if *flOutput != "" {
		f, err := os.Create(*flOutput)
		if err != nil {
			return fmt.Errorf("creating output file: %w", err)
		}
		defer f.Close()
		out = f
	}

	ctx := context.Background()
	var hadMissingOrBlank, hadValidationFailure bool
	for _, plugin := range plugins {
		tbl, ok := plugin.(*osquerytable.Plugin)
		if !ok {
			continue
		}
		spec := tbl.Spec()

		specBytes, err := json.Marshal(spec)
		if err != nil {
			hadValidationFailure = true
			slogger.Log(ctx, slog.LevelWarn,
				"failed to marshal spec",
				"name", tbl.Name(),
				"err", err,
			)
			continue
		}

		for field, validator := range requiredFieldValidators {
			if !validator(spec) {
				hadMissingOrBlank = true
				slogger.Log(ctx, slog.LevelWarn,
					"spec missing or blank required field",
					"name", tbl.Name(),
					"field", field,
				)
			}
		}

		if *flDebug {
			slogger.Log(ctx, slog.LevelDebug, "printing spec", "table", tbl.Name())
		}

		if !*flQuiet {
			fmt.Fprintln(out, string(specBytes))
		}
	}

	if hadValidationFailure {
		return errors.New("one or more specs failed validation")
	}
	if hadMissingOrBlank && !*flMissingOk {
		return errors.New("one or more required fields were missing or blank")
	}
	return nil
}

// runMergeSpecs combines the per-platform spec files in inputPaths into a single
// JSON array (the shape k2 ingests), deduplicating tables by name and unioning
// their platforms. Output is sorted by name and written to outputPath, or stdout
// when empty.
func runMergeSpecs(inputPaths []string, outputPath string) error {
	if len(inputPaths) == 0 {
		return errors.New("merge mode requires one or more spec files as arguments")
	}

	merged := make(map[string]osquerytable.OsqueryTableSpec)
	var conflicts []string
	for _, inputPath := range inputPaths {
		fileConflicts, err := mergeSpecFile(inputPath, merged)
		if err != nil {
			return fmt.Errorf("merging %s: %w", inputPath, err)
		}
		conflicts = append(conflicts, fileConflicts...)
	}

	// A table seen on multiple platforms must expose the same columns everywhere;
	// otherwise the unioned entry would advertise columns a platform lacks. Fail
	// so the divergence is fixed at the source rather than shipped to k2.
	if len(conflicts) > 0 {
		slices.Sort(conflicts)
		return fmt.Errorf("cross-platform table schema mismatch:\n\t%s", strings.Join(conflicts, "\n\t"))
	}

	combined := slices.Collect(maps.Values(merged))
	slices.SortFunc(combined, func(a, b osquerytable.OsqueryTableSpec) int {
		return strings.Compare(a.Name, b.Name)
	})

	combinedBytes, err := json.MarshalIndent(combined, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling combined specs: %w", err)
	}

	out := io.Writer(os.Stdout)
	if outputPath != "" {
		f, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("creating output file: %w", err)
		}
		defer f.Close()
		out = f
	}

	if _, err := fmt.Fprintln(out, string(combinedBytes)); err != nil {
		return fmt.Errorf("writing combined specs: %w", err)
	}

	return nil
}

// mergeSpecFile folds the tables from one spec file into merged, unioning
// platforms for tables already seen and returning any column-schema conflicts
// (see schemaConflicts). The file may be NDJSON or a JSON array (see readSpecs).
func mergeSpecFile(inputPath string, merged map[string]osquerytable.OsqueryTableSpec) ([]string, error) {
	f, err := os.Open(inputPath)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	specs, err := readSpecs(f)
	if err != nil {
		return nil, fmt.Errorf("reading specs: %w", err)
	}

	var conflicts []string
	for _, spec := range specs {
		existing, ok := merged[spec.Name]
		if !ok {
			merged[spec.Name] = spec
			continue
		}

		conflicts = append(conflicts, schemaConflicts(existing, spec)...)
		// Only platforms are merged; description/url/notes/examples keep the
		// first-seen file's values, so divergence there is resolved by input order.
		existing.Platforms = unionPlatforms(existing.Platforms, spec.Platforms)
		merged[spec.Name] = existing
	}

	return conflicts, nil
}

// readSpecs decodes table specs from r, accepting either NDJSON (one spec per
// line, from `launcher specs`) or a single JSON array (from `launcher specs
// --merge`), in compact or pretty-printed form. The shape is detected from the
// first non-whitespace byte.
func readSpecs(r io.Reader) ([]osquerytable.OsqueryTableSpec, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading input: %w", err)
	}

	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, nil
	}

	if data[0] == '[' {
		var specs []osquerytable.OsqueryTableSpec
		if err := json.Unmarshal(data, &specs); err != nil {
			return nil, fmt.Errorf("decoding JSON array of specs: %w", err)
		}
		return specs, nil
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	var specs []osquerytable.OsqueryTableSpec
	for {
		var spec osquerytable.OsqueryTableSpec
		if err := dec.Decode(&spec); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decoding spec near byte offset %d: %w", dec.InputOffset(), err)
		}
		specs = append(specs, spec)
	}

	return specs, nil
}

// schemaConflicts returns descriptions of any column differences between two
// specs for the same table seen on different platforms. Columns are matched by
// name; a conflict is a column on one side but not the other, or a type
// mismatch. Platform lists and doc fields (description/notes) are not compared.
func schemaConflicts(a, b osquerytable.OsqueryTableSpec) []string {
	aCols := columnsByName(a.Columns)
	bCols := columnsByName(b.Columns)

	var conflicts []string
	for name := range aCols {
		if _, ok := bCols[name]; !ok {
			conflicts = append(conflicts, fmt.Sprintf("table %q: column %q present on %v but not %v", a.Name, name, a.Platforms, b.Platforms))
		}
	}
	for name := range bCols {
		if _, ok := aCols[name]; !ok {
			conflicts = append(conflicts, fmt.Sprintf("table %q: column %q present on %v but not %v", a.Name, name, b.Platforms, a.Platforms))
		}
	}
	for name, aCol := range aCols {
		bCol, ok := bCols[name]
		if !ok {
			continue
		}
		if aCol.Type != bCol.Type {
			conflicts = append(conflicts, fmt.Sprintf("table %q: column %q is type %q on %v but %q on %v", a.Name, name, aCol.Type, a.Platforms, bCol.Type, b.Platforms))
		}
	}

	return conflicts
}

func columnsByName(cols []osquerytable.ColumnDefinition) map[string]osquerytable.ColumnDefinition {
	byName := make(map[string]osquerytable.ColumnDefinition, len(cols))
	for _, col := range cols {
		byName[col.Name] = col
	}
	return byName
}

// unionPlatforms returns the de-duplicated union of two platform slices,
// preserving first-seen order. Generic over the unexported platformName type.
func unionPlatforms[T ~string](existing, additional []T) []T {
	seen := make(map[T]struct{}, len(existing)+len(additional))
	union := make([]T, 0, len(existing)+len(additional))
	for _, platforms := range [][]T{existing, additional} {
		for _, platform := range platforms {
			if _, ok := seen[platform]; ok {
				continue
			}
			seen[platform] = struct{}{}
			union = append(union, platform)
		}
	}
	return union
}
