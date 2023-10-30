package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/env"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/launcher/pkg/autoupdate/tuf"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/pkg/execwrapper"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/log/locallogger"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/log/teelogger"
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
		if tuf.UsingNewAutoupdater() {
			runNewerLauncherIfAvailable(ctx, logger)
		} else {
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
	}

	// if the launcher is being ran with a positional argument,
	// handle that argument. Fall-back to running launcher
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], `-`) {
		if err := runSubcommands(); err != nil {
			logutil.Fatal(logger, "err", fmt.Errorf("run with positional args: %w", err))
		}
		os.Exit(0)
	}

	opts, err := launcher.ParseOptions("", os.Args[1:])
	if err != nil {
		level.Info(logger).Log("err", err)
		os.Exit(1)
	}

	// recreate the logger with  the appropriate level.
	logger = logutil.NewServerLogger(opts.Debug)

	// set up system slogger to write to os logs
	systemSlogger := new(multislogger.MultiSlogger)
	systemSlogger.AddHandler(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// set up local slogger to write to debug.json
	localSlogger := new(multislogger.MultiSlogger)

	// Create a local logger. This logs to a known path, and aims to help diagnostics
	if opts.RootDirectory != "" {
		localLogger := locallogger.NewKitLogger(filepath.Join(opts.RootDirectory, "debug.json"))
		logger = teelogger.New(logger, localLogger)

		localSloggerHandler := slog.NewJSONHandler(localLogger.Writer(), &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		})

		localSlogger.AddHandler(localSloggerHandler)

		// also send system logs to localSloggerHandler
		systemSlogger.AddHandler(localSloggerHandler)

		locallogger.CleanUpRenamedDebugLogs(opts.RootDirectory, logger)
	}

	defer func() {
		if r := recover(); r != nil {
			level.Info(logger).Log(
				"msg", "panic occurred",
				"err", r,
			)
			time.Sleep(time.Second)
		}
	}()

	ctx = ctxlog.NewContext(ctx, logger)

	if err := runLauncher(ctx, cancel, localSlogger, systemSlogger, opts); err != nil {
		if tuf.IsLauncherReloadNeededErr(err) {
			level.Debug(logger).Log("msg", "runLauncher exited to run newer version of launcher", "err", err.Error())
			runNewerLauncherIfAvailable(ctx, logger)
		} else {
			level.Debug(logger).Log("msg", "run launcher", "stack", fmt.Sprintf("%+v", err))
			logutil.Fatal(logger, err, "run launcher")
		}
	}
}

func runSubcommands() error {
	var run func([]string) error
	switch os.Args[1] {
	case "socket":
		run = runSocket
	case "query":
		run = runQuery
	case "doctor":
		run = runDoctor
	case "flare":
		run = runFlare
	case "svc":
		run = runWindowsSvc
	case "svc-fg":
		run = runWindowsSvcForeground
	case "version":
		run = runVersion
	case "compactdb":
		run = runCompactDb
	case "interactive":
		run = runInteractive
	case "desktop":
		run = runDesktop
	case "download-osquery":
		run = runDownloadOsquery
	case "uninstall":
		run = runUninstall
	default:
		return fmt.Errorf("Unknown subcommand %s", os.Args[1])
	}

	if err := run(os.Args[2:]); err != nil {
		return fmt.Errorf("running subcommand %s: %w", os.Args[1], err)
	}

	return nil

}

// runNewerLauncherIfAvailable checks the autoupdate library for a newer version
// of launcher than the currently-running one. If found, it will exec that version.
func runNewerLauncherIfAvailable(ctx context.Context, logger log.Logger) {
	newerBinary, err := latestLauncherPath(ctx, logger)
	if err != nil {
		level.Error(logger).Log(
			"msg", "could not check out latest launcher, will fall back to old autoupdate library",
			"err", err,
		)

		// Fall back to legacy autoupdate library
		newerBinary, err = autoupdate.FindNewestSelf(ctx)
		if err != nil {
			return
		}
	}

	if newerBinary == "" {
		level.Info(logger).Log("msg", "nothing newer")
		return
	}

	level.Info(logger).Log(
		"msg", "preparing to exec new binary",
		"old_version", version.Version().Version,
		"new_binary", newerBinary,
	)

	if err := execwrapper.Exec(ctx, newerBinary, os.Args, os.Environ()); err != nil {
		logutil.Fatal(logger, "msg", "error execing newer version of launcher", "new_binary", newerBinary, "err", err)
	}

	logutil.Fatal(logger, "msg", "execing newer version of launcher exited unexpectedly without error", "new_binary", newerBinary)
}

// latestLauncherPath looks for the latest version of launcher in the new autoupdate library,
// falling back to the old library if necessary.
func latestLauncherPath(ctx context.Context, logger log.Logger) (string, error) {
	newerBinary, err := tuf.CheckOutLatestWithoutConfig("launcher", logger)
	if err != nil {
		return "", fmt.Errorf("checking out latest launcher: %w", err)
	}

	currentPath, _ := os.Executable()
	if newerBinary.Version != version.Version().Version && newerBinary.Path != currentPath {
		level.Info(logger).Log(
			"msg", "got new version of launcher to run",
			"old_version", version.Version().Version,
			"new_binary_version", newerBinary.Version,
			"new_binary_path", newerBinary.Path,
		)
		return newerBinary.Path, nil
	}

	// Already running latest version, nothing to do here
	return "", nil
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
