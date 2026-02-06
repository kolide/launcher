package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"

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

func runSpecs(systemMultiSlogger *multislogger.MultiSlogger, args []string) error {
	flagset := flag.NewFlagSet("launcher specs", flag.ExitOnError)
	flDebug := flagset.Bool("debug", false, "enable debug logging")
	var flRequired requiredFields
	flagset.Var(&flRequired, "required", "field name that must be present in the spec (repeatable); warns if missing")

	if err := ff.Parse(flagset, args, ff.WithEnvVarNoPrefix()); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	slogLevel := slog.LevelInfo
	if *flDebug {
		slogLevel = slog.LevelDebug
	}

	systemMultiSlogger.AddHandler(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slogLevel,
		AddSource: true,
	}))

	launcher.SetDefaultPaths()
	opts := &launcher.Options{
		RootDirectory: launcher.DefaultPath(launcher.RootDirectory),
		OsquerydPath:  "",
	}
	flagController := flags.NewFlagController(systemMultiSlogger.Logger, nil, flags.WithCmdLineOpts(opts))
	k := knapsack.New(nil, flagController, nil, nil, nil)
	slogger := systemMultiSlogger.Logger.With("subprocess", "specs")

	launcherTables := table.LauncherTables(k, slogger)
	platformTables := table.PlatformTables(k, "", slogger, "")

	plugins := make([]osquery.OsqueryPlugin, 0, len(launcherTables)+len(platformTables))
	plugins = append(plugins, launcherTables...)
	plugins = append(plugins, platformTables...)

	ctx := context.Background()
	for _, plugin := range plugins {
		tbl, ok := plugin.(*osquerytable.Plugin)
		if !ok {
			continue
		}
		spec, err := tbl.Spec()
		if err != nil {
			slogger.Log(ctx, slog.LevelWarn,
				"table Spec() failed, skipping",
				"name", tbl.Name(),
				"err", err,
			)
			continue
		}

		// Validate spec is valid JSON; also used for required-field checks.
		var specMap map[string]interface{}
		if err := json.Unmarshal(spec, &specMap); err != nil {
			slogger.Log(ctx, slog.LevelWarn,
				"spec is not valid JSON, skipping",
				"name", tbl.Name(),
				"err", err,
			)
			continue
		}
		for _, field := range flRequired {
			if _, ok := specMap[field]; !ok {
				slogger.Log(ctx, slog.LevelWarn,
					"spec missing required field",
					"name", tbl.Name(),
					"field", field,
				)
			}
		}

		if *flDebug {
			slogger.Log(ctx, slog.LevelDebug, "printing spec", "table", tbl.Name())
		}
		fmt.Println(string(spec))
	}

	return nil
}
