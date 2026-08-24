package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/kolide/launcher/v2/ee/agent/flags"
	"github.com/kolide/launcher/v2/ee/agent/knapsack"
	"github.com/kolide/launcher/v2/ee/debug/checkups"
	"github.com/kolide/launcher/v2/pkg/launcher"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
)

func runDoctor(systemMultiSlogger *multislogger.MultiSlogger, args []string) error {
	attachConsole()
	defer detachConsole()

	// Doctor assumes a launcher installation (at least partially) exists
	// Overriding some of the default values allows options to be parsed making this assumption
	launcher.DefaultAutoupdate = true
	launcher.SetDefaultPaths()

	slogLevel := new(slog.LevelVar)
	slogLevel.Set(slog.LevelInfo)
	// Add handler to write to stdout
	systemMultiSlogger.AddHandler(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slogLevel,
		AddSource: true,
	}))

	opts, err := launcher.ParseOptions(systemMultiSlogger.Logger, "doctor", os.Args[2:])
	if err != nil {
		return err
	}

	if opts.Debug {
		slogLevel.Set(slog.LevelDebug)
	}

	fcOpts := []flags.Option{flags.WithCmdLineOpts(opts)}

	flagController := flags.NewFlagController(systemMultiSlogger.Logger, nil, fcOpts...)
	k := knapsack.New(nil, flagController, nil, nil, nil)

	w := os.Stdout //tabwriter.NewWriter(os.Stdout, 0, 8, 1, '\t', tabwriter.AlignRight)

	ctx := context.Background()
	checkups.RunDoctor(ctx, k, w)

	return nil
}
