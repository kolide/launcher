//go:build windows
// +build windows

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	launcherRestartServiceName string = `LauncherKolideRestartSvc`
)

type winRestartSvc struct {
	systemSlogger *multislogger.MultiSlogger
}

func runRestartService(systemSlogger *multislogger.MultiSlogger, args []string) error {
	logCtx := context.TODO()
	systemSlogger.Logger = systemSlogger.Logger.With(
		"service", launcherRestartServiceName,
		"version", version.Version().Version,
	)

	systemSlogger.Log(logCtx, slog.LevelInfo, "windows restart service start requested")

	// Log panics from the windows service
	defer func() {
		if r := recover(); r != nil {
			systemSlogger.Log(logCtx, slog.LevelError,
				"panic occurred in windows restart service",
				"err", r,
			)
			if err, ok := r.(error); ok {
				systemSlogger.Log(logCtx, slog.LevelError,
					"windows restart service panic stack trace",
					"stack_trace", fmt.Sprintf("%+v", errors.WithStack(err)),
				)
			}
			time.Sleep(time.Second)
		}
	}()

	if err := svc.Run(launcherRestartServiceName, &winRestartSvc{
		systemSlogger: systemSlogger,
	}); err != nil {
		systemSlogger.Log(logCtx, slog.LevelError,
			"error in service run",
			"err", err,
		)
		time.Sleep(time.Second)
		return err
	}

	systemSlogger.Log(logCtx, slog.LevelInfo, "service exited")
	time.Sleep(time.Second)

	return nil
}

func (w *winRestartSvc) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	w.systemSlogger.Log(ctx, slog.LevelInfo, "executing windows service")
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	go func() {
		err := runLauncherRestartService(ctx, w)
		if err != nil {
			w.systemSlogger.Log(ctx, slog.LevelInfo,
				"runLauncherRestartService exited",
				"err", err,
				"stack_trace", fmt.Sprintf("%+v", errors.WithStack(err)),
			)
			changes <- svc.Status{State: svc.Stopped, Accepts: cmdsAccepted}
			// Launcher is already shut down -- fully exit so that the service manager can restart the service
			os.Exit(1)
		}

		w.systemSlogger.Log(ctx, slog.LevelInfo, "runLauncherRestartService exited cleanly")
		changes <- svc.Status{State: svc.Stopped, Accepts: cmdsAccepted}
		os.Exit(0)
	}()

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
				time.Sleep(2 * time.Second) // give checker routine enough time to shut down
				changes <- svc.Status{State: svc.Stopped, Accepts: cmdsAccepted}
				return ssec, errno
			default:
				w.systemSlogger.Log(ctx, slog.LevelInfo,
					"unexpected change request",
					"service", launcherRestartServiceName,
					"change_request", fmt.Sprintf("%+v", c),
				)
			}
		}
	}
}

func (w *winRestartSvc) checkLauncherStatus(ctx context.Context) error {
	serviceManager, err := mgr.Connect()
	if err != nil {
		w.systemSlogger.Log(context.TODO(), slog.LevelError,
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

	if currentStatus.State == svc.Stopped { // todo think about what states we actually care about
		w.systemSlogger.Log(ctx, slog.LevelInfo, "restart service checker detected stopped state, restarting")
		launcherService.Start()
	}

	return nil
}

func runLauncherRestartService(ctx context.Context, w *winRestartSvc) error {
	ticker := time.NewTicker(30 * time.Second)

	for {
		select {
		case <-ticker.C:
			w.checkLauncherStatus(ctx)
		case <-ctx.Done():
			ticker.Stop()
			return ctx.Err()
		}
	}
}
