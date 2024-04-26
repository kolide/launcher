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
	"github.com/kolide/launcher/ee/tuf"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/pkg/execwrapper"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/log/locallogger"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/log/teelogger"
	"github.com/pkg/errors"
)

func main() {
	if err := runMain(); err != nil {
		os.Exit(1) //nolint:forbidigo // Our only allowed usages of os.Exit are in this function
	}
	os.Exit(0) //nolint:forbidigo // Our only allowed usages of os.Exit are in this function
}

func runMain() error {
	systemSlogger, logCloser, err := multislogger.SystemSlogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating system logger: %v\n", err)
		return fmt.Errorf("creating system logger: %w", err)
	}
	defer logCloser.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	systemSlogger.Log(ctx, slog.LevelInfo,
		"launcher starting up",
		"version", version.Version().Version,
		"revision", version.Version().Revision,
	)

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
			return fmt.Errorf("running newer version of launcher: %w", err)
		}
	}

	// if the launcher is being ran with a positional argument,
	// handle that argument. Fall-back to running launcher
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], `-`) {
		if err := runSubcommands(systemSlogger); err != nil {
			systemSlogger.Log(ctx, slog.LevelError,
				"running with positional args",
				"err", err,
			)
			return fmt.Errorf("running with positional args: %w", err)
		}
		return nil
	}

	opts, err := launcher.ParseOptions("", os.Args[1:])
	if err != nil {
		if launcher.IsInfoCmd(err) {
			return nil
		}
		systemSlogger.Log(ctx, slog.LevelError,
			"could not parse options",
			"err", err,
		)
		return fmt.Errorf("parsing options: %w", err)
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
		if !tuf.IsLauncherReloadNeededErr(err) {
			level.Debug(logger).Log("msg", "run launcher", "stack", fmt.Sprintf("%+v", err))
			return fmt.Errorf("running launcher: %w", err)
		}
		level.Debug(logger).Log("msg", "runLauncher exited to run newer version of launcher", "err", err.Error())
		if err := runNewerLauncherIfAvailable(ctx, slogger.Logger); err != nil {
			return fmt.Errorf("running newer version of launcher: %w", err)
		}
	}
	return nil
}

func runSubcommands(systemMultiSlogger *multislogger.MultiSlogger) error {
	var run func(*multislogger.MultiSlogger, []string) error
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
	case "secure-enclave":
		run = runSecureEnclave
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
			"could not check out latest launcher, will fall back to old autoupdate library",
			"err", err,
		)

		// Fall back to legacy autoupdate library
		newerBinary, err = autoupdate.FindNewestSelf(ctx)
		if err != nil {
			slogger.Log(ctx, slog.LevelError,
				"could not check out latest launcher from legacy autoupdate library",
				"err", err,
			)
			return nil
		}
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
