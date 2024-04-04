package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/kolide/launcher/ee/agent/flags"
	"github.com/kolide/launcher/ee/agent/knapsack"
	"github.com/kolide/launcher/ee/debug/checkups"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/log/multislogger"
)

func runDoctor(systemMultiSlogger *multislogger.MultiSlogger, args []string) error {
	attachConsole()
	defer detachConsole()

	// Doctor assumes a launcher installation (at least partially) exists
	// Overriding some of the default values allows options to be parsed making this assumption
	launcher.DefaultAutoupdate = true
	launcher.SetDefaultPaths()

	opts, err := launcher.ParseOptions("doctor", os.Args[2:])
	if err != nil {
		return err
	}

	fcOpts := []flags.Option{flags.WithCmdLineOpts(opts)}

	slogLevel := slog.LevelInfo
	if opts.Debug {
		slogLevel = slog.LevelDebug
	}

	// Add handler to write to stdout
	systemMultiSlogger.AddHandler(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     slogLevel,
		AddSource: true,
	}))

	flagController := flags.NewFlagController(systemMultiSlogger.Logger, nil, fcOpts...)
	k := knapsack.New(nil, flagController, nil, nil, nil)

	w := os.Stdout //tabwriter.NewWriter(os.Stdout, 0, 8, 1, '\t', tabwriter.AlignRight)

	ctx := context.Background()
	checkups.RunDoctor(ctx, k, w)

	return nil
}
