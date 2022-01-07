package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/env"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/pkg/execwrapper"
	"github.com/kolide/launcher/pkg/log/locallogger"
	"github.com/kolide/launcher/pkg/log/teelogger"
	"github.com/pkg/errors"
)

func main() {
	// create initial logger. As this is prior to options parsing,
	// use the environment to determine verbosity.  It will be
	// re-leveled during options parsing.
	logger := logutil.NewServerLogger(env.Bool("LAUNCHER_DEBUG", false))

	level.Info(logger).Log(
		"msg", "Launcher starting up",
		"version", version.Version().Version,
		"revision", version.Version().Revision,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctx = ctxlog.NewContext(ctx, logger)

	// If there's a newer version of launcher on disk, use it.
	// This does not call DeleteOldUpdates, on the theory that
	// it's better left to the service to handle cleanup. This is
	// a straight forward exec.
	//
	// launcher is _also_ called when we're checking update
	// validity (with autoupdate.checkExecutable). This is
	// somewhat awkward as we end up with extra call layers.
	//
	// Allow a caller to set `LAUNCHER_SKIP_UPDATES` as a way to
	// skip exec'ing an update. This helps prevent launcher from
	// fork-bombing itself. This is an ENV, because there's no
	// good way to pass it through the flags.
	if !env.Bool("LAUNCHER_SKIP_UPDATES", false) {
		newerBinary, err := autoupdate.FindNewestSelf(ctx)
		if err != nil {
			logutil.Fatal(logger, err, "checking for updated version")
		}

		if newerBinary != "" {
			level.Debug(logger).Log(
				"msg", "preparing to exec new binary",
				"oldVersion", version.Version().Version,
				"newBinary", newerBinary,
			)
			if err := execwrapper.Exec(ctx, newerBinary, os.Args, os.Environ()); err != nil {
				logutil.Fatal(logger, err, "exec")
			}
			panic("how")
		} else {
			level.Debug(logger).Log("msg", "Nothing new")
		}
	}

	// if the launcher is being ran with a positional argument,
	// handle that argument. Fall-back to running launcher
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], `-`) {
		if err := runSubcommands(); err != nil {
			logutil.Fatal(logger, "err", errors.Wrap(err, "run with positional args"))
		}
		os.Exit(0)
	}

	opts, err := parseOptions(os.Args[1:])
	if err != nil {
		level.Info(logger).Log("err", err)
		os.Exit(1)
	}

	// recreate the logger with  the appropriate level.
	logger = logutil.NewServerLogger(opts.Debug)

	// Create a local logger. This logs to a known path, and aims to help diagnostics
	if opts.RootDirectory != "" {
		logger = teelogger.New(logger, locallogger.NewKitLogger(filepath.Join(opts.RootDirectory, "debug.log")))
	}

	defer func() {
		if r := recover(); r != nil {
			level.Info(logger).Log(
				"msg", "panic occurred",
				"err", err,
			)
			time.Sleep(time.Second)
		}
	}()

	ctx = ctxlog.NewContext(ctx, logger)

	if err := runLauncher(ctx, cancel, opts); err != nil {
		level.Debug(logger).Log(err, "run launcher", "stack", fmt.Sprintf("%+v", err))
		logutil.Fatal(logger, err, "run launcher")
	}
}

func runSubcommands() error {
	var run func([]string) error
	switch os.Args[1] {
	case "socket":
		run = runSocket
	case "query":
		run = runQuery
	case "flare":
		run = runFlare
	case "svc":
		run = runWindowsSvc
	case "svc-fg":
		run = runWindowsSvcForeground
	case "version":
		run = runVersion
	default:
		return errors.Errorf("Unknown subcommand %s", os.Args[1])
	}

	err := run(os.Args[2:])
	return errors.Wrapf(err, "running subcommand %s", os.Args[1])
}

func commandUsage(fs *flag.FlagSet, short string) func() {
	return func() {
		fmt.Fprintf(os.Stderr, "  Usage:\n")
		fmt.Fprintf(os.Stderr, "    %s\n", short)
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "  Flags:\n")
		w := tabwriter.NewWriter(os.Stderr, 0, 2, 2, ' ', 0)
		fs.VisitAll(func(f *flag.Flag) {
			fmt.Fprintf(w, "    --%s %s\t%s\n", f.Name, f.DefValue, f.Usage)
		})
		w.Flush()
		fmt.Fprintf(os.Stderr, "\n")
	}
}

func runVersion(args []string) error {
	version.PrintFull()
	os.Exit(0)
	return nil
}
