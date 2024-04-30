//go:build windows
// +build windows

package restartservice

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/kolide/kit/version"
	agentsqlite "github.com/kolide/launcher/ee/agent/storage/sqlite"
	"github.com/kolide/launcher/ee/gowrapper"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/log/sqlitelogger"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	LauncherRestartServiceName string = `LauncherKolideRestartSvc`
	launcherServiceName        string = `LauncherKolideK2Svc`
)

type winRestartSvc struct {
	systemSlogger, slogger *multislogger.MultiSlogger
	opts                   *launcher.Options
}

func RunRestartService(systemSlogger *multislogger.MultiSlogger, args []string) error {
	ctx := context.TODO()
	systemSlogger.Logger = systemSlogger.Logger.With(
		"service", LauncherRestartServiceName,
		"version", version.Version().Version,
	)

	systemSlogger.Log(ctx, slog.LevelInfo, "windows restart service start requested")

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
		ll, err := sqlitelogger.NewSqliteLogWriter(ctx, opts.RootDirectory, agentsqlite.RestartServiceLogStore)
		if err != nil {
			return fmt.Errorf("initializing sqlite log writer: %w", err)
		}

		localSloggerHandler := slog.NewJSONHandler(ll, &slog.HandlerOptions{Level: slog.LevelDebug})

		// add the sqlite handler to both local and systemSloggers
		localSlogger.AddHandler(localSloggerHandler)
		systemSlogger.AddHandler(localSloggerHandler)
	}

	localSlogger.Logger = localSlogger.Logger.With(
		"service", LauncherRestartServiceName,
		"version", version.Version().Version,
	)

	// Log panics from the windows service
	defer func() {
		if r := recover(); r != nil {
			systemSlogger.Log(ctx, slog.LevelError,
				"panic occurred in windows restart service",
				"err", r,
			)
			if err, ok := r.(error); ok {
				systemSlogger.Log(ctx, slog.LevelError,
					"windows restart service panic stack trace",
					"stack_trace", fmt.Sprintf("%+v", errors.WithStack(err)),
				)
			}
			time.Sleep(time.Second)
		}
	}()

	if err := svc.Run(LauncherRestartServiceName, &winRestartSvc{
		systemSlogger: systemSlogger,
		slogger:       localSlogger,
		opts:          opts,
	}); err != nil {
		systemSlogger.Log(ctx, slog.LevelError,
			"error in service run",
			"err", err,
		)
		time.Sleep(time.Second)
		return err
	}

	systemSlogger.Log(ctx, slog.LevelInfo, "service exited")
	time.Sleep(time.Second)

	return nil
}

func (w *winRestartSvc) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	w.slogger.Log(ctx, slog.LevelInfo, "executing windows service")
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	runRestartServiceResults := make(chan struct{})

	gowrapper.Go(ctx, w.systemSlogger.Logger, func() {
		err := runLauncherRestartService(ctx, w)
		if err != nil {
			w.systemSlogger.Log(ctx, slog.LevelInfo,
				"runLauncherRestartService exited",
				"err", err,
				"stack_trace", fmt.Sprintf("%+v", errors.WithStack(err)),
			)
		} else {
			w.systemSlogger.Log(ctx, slog.LevelInfo,
				"runLauncher exited cleanly",
			)
		}

		// signal to fully exit so that the service manager can restart the service
		runRestartServiceResults <- struct{}{}
	}, func(r any) {
		w.systemSlogger.Log(ctx, slog.LevelError,
			"exiting after runLauncherRestartService panic",
			"err", r,
		)

		// signal to fully exit so that the service manager can restart the service.
		runRestartServiceResults <- struct{}{}
	})

	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
				// Testing deadlock from https://code.google.com/p/winsvc/issues/detail?id=4
				time.Sleep(100 * time.Millisecond)
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				w.systemSlogger.Log(ctx, slog.LevelInfo, "shutdown request received")
				changes <- svc.Status{State: svc.StopPending}
				cancel()
				time.Sleep(1 * time.Second) // give checker routine enough time to shut down
				changes <- svc.Status{State: svc.Stopped, Accepts: cmdsAccepted}
				return ssec, errno
			default:
				w.systemSlogger.Log(ctx, slog.LevelInfo,
					"unexpected change request",
					"service", LauncherRestartServiceName,
					"change_request", fmt.Sprintf("%+v", c),
				)
			}
		case <-runRestartServiceResults:
			w.systemSlogger.Log(ctx, slog.LevelInfo,
				"shutting down restart service after exit",
			)
			// We don't want to tell the service manager that we've stopped on purpose,
			// so that the service manager will restart launcher correctly.
			// We use this error code largely because the windows/svc code also uses it
			// and it seems semantically correct enough; it doesn't appear to matter to us
			// what the code is.
			return false, uint32(windows.ERROR_EXCEPTION_IN_SERVICE)
		}
	}
}

func (w *winRestartSvc) checkLauncherStatus(ctx context.Context) error {
	serviceManager, err := mgr.Connect()
	if err != nil {
		w.slogger.Log(ctx, slog.LevelError,
			"connecting to service control manager",
			"err", err,
		)

		return err
	}

	defer serviceManager.Disconnect()

	launcherService, err := serviceManager.OpenService(launcherServiceName)
	if err != nil {
		return fmt.Errorf("opening launcher service: %w", err)
	}

	defer launcherService.Close()

	currentStatus, err := launcherService.Query()
	if err != nil {
		return fmt.Errorf("checking current launcher status: %w", err)
	}

	if currentStatus.State == svc.Stopped {
		w.slogger.Log(ctx, slog.LevelInfo, "restart service checker detected stopped state, restarting")
		return launcherService.Start()
	}

	return nil
}

func runLauncherRestartService(ctx context.Context, w *winRestartSvc) error {
	ticker := time.NewTicker(1 * time.Minute)

	for {
		select {
		case <-ticker.C:
			if err := w.checkLauncherStatus(ctx); err != nil {
				w.slogger.Log(ctx, slog.LevelError,
					"failure checking launcher health status",
					"err", err,
				)
			}
		case <-ctx.Done():
			ticker.Stop()
			return ctx.Err()
		}
	}
}
