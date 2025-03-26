//go:build windows
// +build windows

package watchdog

import (
	"context"
	"flag"
	"fmt"
	"log/slog"

	"github.com/kolide/kit/version"
	agentsqlite "github.com/kolide/launcher/ee/agent/storage/sqlite"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/peterbourgon/ff/v3"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// RunWatchdogTask is typically run as a check to determine the health of launcher and restart if required.
// it is installed as an exec action via windows scheduled task. e.g. C:\path\to\launcher.exe watchdog -config <path>.
// you can alternatively run this subcommand to install or remove the scheduled task via the --install-task or --remove-task flags
func RunWatchdogTask(systemSlogger *multislogger.MultiSlogger, args []string) error {
	launcher.DefaultAutoupdate = true
	launcher.SetDefaultPaths()

	var (
		flagset          = flag.NewFlagSet("watchdog", flag.ExitOnError)
		flInstallTask    = flagset.Bool("install-task", false, "install the watchdog as a scheduled task")
		flRemoveTask     = flagset.Bool("remove-task", false, "remove the watchdog as a scheduled task")
		flConfigFilePath = flagset.String("config", launcher.DefaultConfigFilePath, "config file to parse options from (optional)")
	)

	// note that we don't intend to parse the config file here, just the config file path to pass to launcher's ParseOptions
	ff.Parse(flagset, args)

	// pass the config file through our standard options parsing to get all default options
	opts, err := launcher.ParseOptions("watchdog", []string{"-config", *flConfigFilePath})
	if err != nil {
		return fmt.Errorf("parsing watchdog options: %w", err)
	}

	localSlogger := multislogger.New()

	ctx := context.TODO()
	launcherWatchdogTaskName := launcher.TaskName(opts.Identifier, watchdogTaskType)
	systemSlogger.Logger = systemSlogger.Logger.With(
		"task", launcherWatchdogTaskName,
		"version", version.Version().Version,
	)

	// Create a local logger to drop logs into the sqlite DB. These will be collected and published
	// to debug.json from the primary launcher invocation
	if opts.RootDirectory != "" {
		logWriter, err := agentsqlite.OpenRW(ctx, opts.RootDirectory, agentsqlite.WatchdogLogStore)
		if err != nil {
			return fmt.Errorf("opening log db in %s: %w", opts.RootDirectory, err)
		}

		defer logWriter.Close()

		localSloggerHandler := slog.NewJSONHandler(logWriter, &slog.HandlerOptions{Level: slog.LevelDebug})

		// add the sqlite handler to both local and systemSloggers
		localSlogger.AddHandler(localSloggerHandler)
		systemSlogger.AddHandler(localSloggerHandler)
	}

	localSlogger.Logger = localSlogger.Logger.With(
		"task", launcherWatchdogTaskName,
		"version", version.Version().Version,
	)

	if *flInstallTask {
		if err := installWatchdogTask(opts.Identifier, opts.ConfigFilePath); err != nil {
			localSlogger.Log(ctx, slog.LevelWarn,
				"encountered error attempting watchdog install from CLI",
				"err", err,
			)

			return err
		}

		return nil
	}

	if *flRemoveTask {
		if err := RemoveWatchdogTask(opts.Identifier); err != nil {
			localSlogger.Log(ctx, slog.LevelWarn,
				"encountered error attempting watchdog removal from CLI",
				"err", err,
			)

			return err
		}

		return nil
	}

	localSlogger.Log(ctx, slog.LevelDebug, "watchdog check requested")

	launcherServiceName := launcher.ServiceName(opts.Identifier)
	if err := ensureServiceRunning(ctx, localSlogger.Logger, launcherServiceName); err != nil {
		localSlogger.Log(ctx, slog.LevelWarn,
			"encountered error ensuring service run state",
			"err", err,
		)
	}

	return nil
}

func ensureServiceRunning(ctx context.Context, slogger *slog.Logger, serviceName string) error {
	serviceManager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connecting to service control manager: %w", err)
	}

	defer serviceManager.Disconnect()

	launcherService, err := serviceManager.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("opening launcher service: %w", err)
	}

	defer launcherService.Close()

	currentStatus, err := launcherService.Query()
	if err != nil {
		return fmt.Errorf("checking current launcher status: %w", err)
	}

	if currentStatus.State == svc.Stopped {
		slogger.Log(ctx, slog.LevelInfo, "watchdog checker detected stopped state, restarting")
		return launcherService.Start()
	}

	return nil
}
