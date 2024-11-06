//go:build windows
// +build windows

package watchdog

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/kolide/kit/version"
	agentsqlite "github.com/kolide/launcher/ee/agent/storage/sqlite"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

func RunWatchdogTask(systemSlogger *multislogger.MultiSlogger, args []string) error {
	ctx := context.TODO()
	systemSlogger.Logger = systemSlogger.Logger.With(
		"service", launcherWatchdogServiceName,
		"version", version.Version().Version,
	)

	systemSlogger.Log(ctx, slog.LevelDebug, "watchdog check requested")

	opts, err := launcher.ParseOptions("", os.Args[2:])
	if err != nil {
		systemSlogger.Log(ctx, slog.LevelError,
			"error parsing options",
			"err", err,
		)

		return fmt.Errorf("parsing options: %w", err)
	}

	localSlogger := multislogger.New()

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
		"service", launcherWatchdogServiceName,
		"version", version.Version().Version,
	)

	launcherServiceName := launcher.ServiceName(opts.Identifier)
	if err = ensureServiceRunning(ctx, localSlogger.Logger, launcherServiceName); err != nil {
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
