package main

import (
	"context"
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

func runSpecs(systemMultiSlogger *multislogger.MultiSlogger, args []string) error {
	flagset := flag.NewFlagSet("kolide specs", flag.ExitOnError)
	flDebug := flagset.Bool("debug", false, "enable debug logging")
	flErrorOnMissing := flagset.Bool("error-on-missing", false, "for usage later")

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

	_ = flErrorOnMissing // reserved for future use

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
		fmt.Println(string(spec))
	}

	return nil
}
