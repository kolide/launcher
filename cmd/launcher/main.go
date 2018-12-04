// +build !windows

package main

import (
	"context"
	"os"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/kit/version"
	"github.com/pkg/errors"
)

func main() {
	var logger log.Logger
	logger = log.NewJSONLogger(os.Stderr) // only used until options are parsed.

	// if the launcher is being ran with a positional argument, handle that
	// argument. If a known positional argument is not supplied, fall-back to
	// running an osquery instance.
	if isSubCommand() {
		if err := runSubcommands(); err != nil {
			logutil.Fatal(logger, "err", errors.Wrap(err, "run with positional args"))
		}
	}

	opts, err := parseOptions()
	if err != nil {
		level.Info(logger).Log("err", err)
		os.Exit(1)
	}

	// handle --version
	if opts.printVersion {
		version.PrintFull()
		os.Exit(0)
	}

	// handle --usage
	if opts.developerUsage {
		developerUsage()
		os.Exit(0)
	}

	logger = logutil.NewServerLogger(opts.debug)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := runLauncher(ctx, cancel, opts, logger); err != nil {
		logutil.Fatal(logger, err, "run launcher")
	}
}

func isSubCommand() bool {
	if len(os.Args) > 2 {
		return false
	}

	subCommands := []string{
		"socket",
		"query",
		"flare",
	}

	for _, sc := range subCommands {
		if sc == os.Args[1] {
			return true
		}
	}

	return false
}
