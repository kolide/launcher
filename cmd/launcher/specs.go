package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"

	"github.com/kolide/launcher/ee/agent/flags"
	"github.com/kolide/launcher/ee/agent/knapsack"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/osquery/table"
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

// specFieldBlank returns true if the field is missing, nil, or empty (e.g. "" or []).
func specFieldBlank(v interface{}) bool {
	if v == nil {
		return true
	}
	switch x := v.(type) {
	case string:
		return len(x) < 1
	case []interface{}:
		return len(x) < 1
	case map[string]interface{}:
		return len(x) < 1
	default:
		return false
	}
}

func runSpecs(systemMultiSlogger *multislogger.MultiSlogger, args []string) error {
	flagset := flag.NewFlagSet("launcher specs", flag.ExitOnError)
	flDebug := flagset.Bool("debug", false, "enable debug logging")
	flQuiet := flagset.Bool("quiet", false, "don't print specs. Used in testing")
	flOutput := flagset.String("output", "", "write specs to file (default: stdout)")
	flMissingOk := flagset.Bool("missing-ok", false, "do not exit with error when required fields are missing or blank")
	var flRequired requiredFields
	flagset.Var(&flRequired, "required", "field name that must be present in the spec (repeatable); warns if missing")

	if err := ff.Parse(flagset, args, ff.WithEnvVarNoPrefix()); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	slogLevel := slog.LevelInfo
	if *flDebug {
		slogLevel = slog.LevelDebug
	}

	// On Windows, use stdout for log output because stderr is not available.
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

		// Required-field checks use the marshaled map (same field names as JSON).
		var specMap map[string]interface{}
		if err := json.Unmarshal(specBytes, &specMap); err != nil {
			hadValidationFailure = true
			slogger.Log(ctx, slog.LevelWarn,
				"failed to unmarshal spec for required check",
				"name", tbl.Name(),
				"err", err,
			)
			continue
		}
		for _, field := range flRequired {
			if specFieldBlank(specMap[field]) {
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
