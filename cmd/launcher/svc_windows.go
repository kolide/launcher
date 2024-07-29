//go:build windows
// +build windows

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/logutil"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/ee/gowrapper"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/log/locallogger"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
)

// TODO This should be inherited from some setting
const serviceName = "launcher"

// runWindowsSvc starts launcher as a windows service. This will
// probably not behave correctly if you start it from the command line.
// This method is responsible for calling svc.Run, which eventually translates into the
// Execute function below. Each device has a global ServicesPipeTimeout value (typically at
// 30-45 seconds but depends on the configuration of the device). We have that many seconds
// to get from here to the point in Execute where we return a service status of Running before
// service control manager will consider the start attempt to have timed out, cancel it,
// and proceed without attempting restart.
// Wherever possible, we should keep any connections or timely operations out of this method,
// and ensure they are added late enough in Execute to avoid hitting this timeout.
func runWindowsSvc(systemSlogger *multislogger.MultiSlogger, args []string) error {
	systemSlogger.Log(context.TODO(), slog.LevelInfo,
		"service start requested",
		"version", version.Version().Version,
	)

	opts, err := launcher.ParseOptions("", os.Args[2:])
	if err != nil {
		systemSlogger.Log(context.TODO(), slog.LevelInfo,
			"error parsing options",
			"err", err,
		)
		return fmt.Errorf("parsing options: %w", err)
	}

	localSlogger := multislogger.New()
	logger := log.NewNopLogger()

	if opts.RootDirectory != "" {
		// Create a local logger. This logs to a known path, and aims to help diagnostics
		ll := locallogger.NewKitLogger(filepath.Join(opts.RootDirectory, "debug.json"))
		logger = ll

		localSloggerHandler := slog.NewJSONHandler(ll.Writer(), &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		})

		localSlogger.AddHandler(localSloggerHandler)

		// also write system logs to localSloggerHandler
		systemSlogger.AddHandler(localSloggerHandler)
	}

	systemSlogger.Log(context.TODO(), slog.LevelInfo,
		"launching service",
		"version", version.Version().Version,
	)

	// Log panics from the windows service
	defer func() {
		if r := recover(); r != nil {
			systemSlogger.Log(context.TODO(), slog.LevelInfo,
				"panic occurred in windows service",
				"err", r,
			)
			if err, ok := r.(error); ok {
				systemSlogger.Log(context.TODO(), slog.LevelInfo,
					"panic stack trace",
					"stack_trace", fmt.Sprintf("%+v", errors.WithStack(err)),
				)
			}
			time.Sleep(time.Second)
		}
	}()

	if err := svc.Run(serviceName, &winSvc{
		logger:        logger,
		slogger:       localSlogger,
		systemSlogger: systemSlogger,
		opts:          opts,
	}); err != nil {
		// TODO The caller doesn't have the event log configured, so we
		// need to log here. this implies we need some deeper refactoring
		// of the logging
		systemSlogger.Log(context.TODO(), slog.LevelInfo,
			"error in service run",
			"err", err,
		)
		time.Sleep(time.Second)
		return err
	}

	systemSlogger.Log(context.TODO(), slog.LevelInfo,
		"service exited",
	)

	time.Sleep(time.Second)

	return nil
}

func runWindowsSvcForeground(systemSlogger *multislogger.MultiSlogger, args []string) error {
	attachConsole()
	defer detachConsole()

	// Foreground mode is inherently a debug mode. So we start the
	// logger in debugging mode, instead of looking at opts.debug
	logger := logutil.NewCLILogger(true)
	level.Debug(logger).Log("msg", "foreground service start requested (debug mode)")

	// Use new logger to write logs to stdout
	localSlogger := new(multislogger.MultiSlogger)

	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	})
	localSlogger.AddHandler(handler)
	systemSlogger.AddHandler(handler)

	opts, err := launcher.ParseOptions("", os.Args[2:])
	if err != nil {
		level.Info(logger).Log("err", err)
		return fmt.Errorf("parsing options: %w", err)
	}

	// set extra debug options
	opts.Debug = true
	opts.OsqueryVerbose = true

	run := debug.Run

	return run(serviceName, &winSvc{logger: logger, slogger: localSlogger, systemSlogger: systemSlogger, opts: opts})
}

type winSvc struct {
	logger                 log.Logger
	slogger, systemSlogger *multislogger.MultiSlogger
	opts                   *launcher.Options
}

func (w *winSvc) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	w.systemSlogger.Log(ctx, slog.LevelInfo,
		"windows service starting",
	)
	// after this point windows service control manager will know that we've successfully started,
	// it is safe to begin longer running operations
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	// Confirm that service configuration is up-to-date
	go checkServiceConfiguration(w.slogger.Logger, w.opts)

	ctx = ctxlog.NewContext(ctx, w.logger)
	runLauncherResults := make(chan struct{})

	gowrapper.Go(ctx, w.systemSlogger.Logger, func() {
		err := runLauncher(ctx, cancel, w.slogger, w.systemSlogger, w.opts)
		if err != nil {
			w.systemSlogger.Log(ctx, slog.LevelInfo,
				"runLauncher exited",
				"err", err,
				"stack_trace", fmt.Sprintf("%+v", errors.WithStack(err)),
			)
		} else {
			w.systemSlogger.Log(ctx, slog.LevelInfo,
				"runLauncher exited cleanly",
			)
		}

		// Since launcher shut down, we must signal to fully exit so that the service manager can restart the service.
		runLauncherResults <- struct{}{}
	}, func(r any) {
		w.systemSlogger.Log(ctx, slog.LevelError,
			"exiting after runLauncher panic",
			"err", r,
		)
		// Since launcher shut down, we must signal to fully exit so that the service manager can restart the service.
		runLauncherResults <- struct{}{}
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
				w.systemSlogger.Log(ctx, slog.LevelInfo,
					"shutdown request received",
				)
				changes <- svc.Status{State: svc.StopPending}
				cancel()
				time.Sleep(2 * time.Second) // give rungroups enough time to shut down
				changes <- svc.Status{State: svc.Stopped, Accepts: cmdsAccepted}
				return ssec, errno
			default:
				w.systemSlogger.Log(ctx, slog.LevelInfo,
					"unexpected change request",
					"change_request", fmt.Sprintf("%+v", c),
				)
			}
		case <-runLauncherResults:
			w.systemSlogger.Log(ctx, slog.LevelInfo,
				"shutting down service after runLauncher exited",
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
