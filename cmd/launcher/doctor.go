package main

import (
	"context"
	"os"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/launcher/pkg/agent/flags"
	"github.com/kolide/launcher/pkg/agent/knapsack"
	"github.com/kolide/launcher/pkg/debug/checkups"
	"github.com/kolide/launcher/pkg/launcher"
)

func runDoctor(args []string) error {
	// Doctor assumes a launcher installation (at least partially) exists
	// Overriding some of the default values allows options to be parsed making this assumption
	launcher.DefaultAutoupdate = true
	launcher.SetDefaultPaths()

	opts, err := launcher.ParseOptions("doctor", os.Args[2:])
	if err != nil {
		return err
	}

	fcOpts := []flags.Option{flags.WithCmdLineOpts(opts)}
	logger := log.With(logutil.NewCLILogger(true), "caller", log.DefaultCaller)
	flagController := flags.NewFlagController(logger, nil, fcOpts...)
	k := knapsack.New(nil, flagController, nil, nil)

	w := os.Stdout //tabwriter.NewWriter(os.Stdout, 0, 8, 1, '\t', tabwriter.AlignRight)

	ctx := context.Background()
	checkups.RunDoctor(ctx, k, w)

	return nil
}
