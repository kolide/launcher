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

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/env"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/ee/control/consumers/remoterestartconsumer"
	"github.com/kolide/launcher/ee/disclaim"
	"github.com/kolide/launcher/ee/tuf"
	"github.com/kolide/launcher/ee/watchdog"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/pkg/execwrapper"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/log/locallogger"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/log/teelogger"
	"github.com/pkg/errors"
)

func main() {
	os.Exit(runMain()) //nolint:forbidigo // Our only allowed usage of os.Exit is in this function
}

// runMain runs launcher main -- selecting the appropriate subcommand and running that (if provided),
// or running `runLauncher`. We wrap it so that all our deferred calls will execute before we call
// `os.Exit` in `main()`.
func runMain() int {
	systemSlogger, logCloser, err := multislogger.SystemSlogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating system logger: %v\n", err)
		return 1
	}
	defer logCloser.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	systemSlogger.Log(ctx, slog.LevelInfo,
		"launcher starting up",
		"version", version.Version().Version,
		"revision", version.Version().Revision,
	)

	// Set an os environmental variable that we can use to track launcher versions across
	// various bits of updated binaries
	if chain := os.Getenv("KOLIDE_LAUNCHER_VERSION_CHAIN"); chain == "" {
		os.Setenv("KOLIDE_LAUNCHER_VERSION_CHAIN", version.Version().Version)
	} else {
		os.Setenv("KOLIDE_LAUNCHER_VERSION_CHAIN", fmt.Sprintf("%s:%s", chain, version.Version().Version))
	}

	// create initial logger. As this is prior to options parsing,
	// use the environment to determine verbosity.  It will be
	// re-leveled during options parsing.
	logger := logutil.NewServerLogger(env.Bool("LAUNCHER_DEBUG", false))
	ctx = ctxlog.NewContext(ctx, logger)

	// If this is a development build directory, we want to skip the TUF lookups and just run
	// the requested build. Don't care about errors here. This is developer experience shim
	inBuildDir := false
	if execPath, err := os.Executable(); err == nil {
		inBuildDir = strings.Contains(execPath, filepath.Join("launcher", "build")) && !env.Bool("LAUNCHER_FORCE_UPDATE_IN_BUILD", false)
	}

	// If there's a newer version of launcher on disk, use it.
	// Allow a caller to set `LAUNCHER_SKIP_UPDATES` as a way to
	// skip exec'ing an update. This helps prevent launcher from
	// fork-bombing itself. This is an ENV, because there's no
	// good way to pass it through the flags.
	if !env.Bool("LAUNCHER_SKIP_UPDATES", false) && !inBuildDir {
		if err := runNewerLauncherIfAvailable(ctx, systemSlogger.Logger); err != nil {
			systemSlogger.Log(ctx, slog.LevelInfo,
				"could not run newer version of launcher",
				"err", err,
			)
			return 1
		}
	}

	// if the launcher is being ran with a positional argument,
	// handle that argument.
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], `-`) {
		if err := runSubcommands(systemSlogger); err != nil {
			systemSlogger.Log(ctx, slog.LevelError,
				"running with positional args",
				"err", err,
			)
			return 1
		}
		return 0
	}

	// Fall back to running launcher
	opts, err := launcher.ParseOptions("", os.Args[1:])
	if err != nil {
		if launcher.IsInfoCmd(err) {
			return 0
		}
		systemSlogger.Log(ctx, slog.LevelError,
			"could not parse options",
			"err", err,
		)
		return 0
	}

	// recreate the logger with  the appropriate level.
	logger = logutil.NewServerLogger(opts.Debug)

	// set up slogger for internal launcher logging
	slogger := multislogger.New()

	// Create a local logger. This logs to a known path, and aims to help diagnostics
	if opts.RootDirectory != "" {
		localLogger := locallogger.NewKitLogger(filepath.Join(opts.RootDirectory, "debug.json"))
		logger = teelogger.New(logger, localLogger)

		localSloggerHandler := slog.NewJSONHandler(localLogger.Writer(), &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		})

		slogger.AddHandler(localSloggerHandler)

		// also send system logs to localSloggerHandler
		systemSlogger.AddHandler(localSloggerHandler)
	}

	defer func() {
		if r := recover(); r != nil {
			level.Info(logger).Log(
				"msg", "panic occurred",
				"err", r,
			)
			if err, ok := r.(error); ok {
				level.Info(logger).Log(
					"msg", "panic stack trace",
					"stack_trace", fmt.Sprintf("%+v", errors.WithStack(err)),
				)
			}
			time.Sleep(time.Second)
		}
	}()

	ctx = ctxlog.NewContext(ctx, logger)

	if err := runLauncher(ctx, cancel, slogger, systemSlogger, opts); err != nil {
		// launcher exited due to error that does not require further handling -- return now so we can exit
		if !tuf.IsLauncherReloadNeededErr(err) && !errors.Is(err, remoterestartconsumer.ErrRemoteRestartRequested) {
			level.Debug(logger).Log("msg", "run launcher", "stack", fmt.Sprintf("%+v", err))
			return 1
		}

		// Autoupdate asked for a restart to run the newly-downloaded version of launcher -- run that newer version
		if tuf.IsLauncherReloadNeededErr(err) {
			level.Debug(logger).Log("msg", "runLauncher exited to load newer version of launcher after autoupdate", "err", err.Error())
			if err := runNewerLauncherIfAvailable(ctx, slogger.Logger); err != nil {
				return 1
			}
		}

		// A remote restart was requested -- run this version of launcher again.
		// We need a full exec of our current executable, rather than just calling runLauncher again.
		// This ensures we don't run into issues where artifacts of our previous runLauncher call
		// stick around (for example, the signal listener panicking on send to closed channel).
		currentExecutable, err := os.Executable()
		if err != nil {
			level.Debug(logger).Log("msg", "could not get current executable to perform remote restart", "err", err.Error())
			return 1
		}
		if err := execwrapper.Exec(ctx, currentExecutable, os.Args, os.Environ()); err != nil {
			slogger.Log(ctx, slog.LevelError,
				"error execing launcher after remote restart was requested",
				"binary", currentExecutable,
				"err", err,
			)
			return 1
		}
	}

	// launcher exited without error -- nothing to do here
	return 0
}

func runSubcommands(systemMultiSlogger *multislogger.MultiSlogger) error {
	var run func(*multislogger.MultiSlogger, []string) error
	switch os.Args[1] {
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
	case "watchdog": // note: this is currently only implemented for windows
		run = watchdog.RunWatchdogTask
	case "query-windowsupdates":
		run = runQueryWindowsUpdates
	case "rundisclaimed":
		run = disclaim.RunDisclaimed
	default:
		return fmt.Errorf("unknown subcommand %s", os.Args[1])
	}

	if err := run(systemMultiSlogger, os.Args[2:]); err != nil {
		return fmt.Errorf("running subcommand %s: %w", os.Args[1], err)
	}

	return nil
}

// runNewerLauncherIfAvailable checks the autoupdate library for a newer version
// of launcher than the currently-running one. If found, it will exec that version.
func runNewerLauncherIfAvailable(ctx context.Context, slogger *slog.Logger) error {
	newerBinary, err := latestLauncherPath(ctx, slogger)
	if err != nil {
		slogger.Log(ctx, slog.LevelError,
			"could not check out latest launcher",
			"err", err,
		)
		return nil
	}

	if newerBinary == "" {
		slogger.Log(ctx, slog.LevelInfo,
			"nothing newer",
		)
		return nil
	}

	slogger.Log(ctx, slog.LevelInfo,
		"preparing to exec new binary",
		"old_version", version.Version().Version,
		"new_binary", newerBinary,
	)

	if err := execwrapper.Exec(ctx, newerBinary, os.Args, os.Environ()); err != nil {
		slogger.Log(ctx, slog.LevelError,
			"error execing newer version of launcher",
			"new_binary", newerBinary,
			"err", err,
		)
		return fmt.Errorf("execing newer version of launcher: %w", err)
	}

	slogger.Log(ctx, slog.LevelError,
		"execing newer version of launcher exited unexpectedly without error",
		"new_binary", newerBinary,
	)
	return errors.New("execing newer version of launcher exited unexpectedly without error")
}

// latestLauncherPath looks for the latest version of launcher in the new autoupdate library,
// falling back to the old library if necessary.
func latestLauncherPath(ctx context.Context, slogger *slog.Logger) (string, error) {
	newerBinary, err := tuf.CheckOutLatestWithoutConfig("launcher", slogger)
	if err != nil {
		return "", fmt.Errorf("checking out latest launcher: %w", err)
	}

	currentPath, _ := os.Executable()
	if newerBinary.Version != version.Version().Version && newerBinary.Path != currentPath {
		slogger.Log(ctx, slog.LevelInfo,
			"got new version of launcher to run",
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

func runVersion(_ *multislogger.MultiSlogger, args []string) error {
	attachConsole()
	version.PrintFull()
	detachConsole()

	return nil
}
