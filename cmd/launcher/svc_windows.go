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
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/kolide/launcher/pkg/contexts/ctxlog"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/kolide/launcher/pkg/log/eventlog"
	"github.com/kolide/launcher/pkg/log/locallogger"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/log/teelogger"
	"github.com/pkg/errors"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
)

// TODO This should be inherited from some setting
const serviceName = "launcher"

// runWindowsSvc starts launcher as a windows service. This will
// probably not behave correctly if you start it from the command line.
func runWindowsSvc(args []string) error {
	eventLogWriter, err := eventlog.NewWriter(serviceName)
	if err != nil {
		return fmt.Errorf("create eventlog writer: %w", err)
	}
	defer eventLogWriter.Close()

	logger := eventlog.New(eventLogWriter)
	logger = log.With(logger, "ts", log.DefaultTimestampUTC, "caller", log.DefaultCaller)

	level.Debug(logger).Log(
		"msg", "service start requested",
		"version", version.Version().Version,
	)

	opts, err := launcher.ParseOptions("", os.Args[2:])
	if err != nil {
		level.Info(logger).Log("msg", "Error parsing options", "err", err)
		os.Exit(1)
	}

	// Now that we've parsed the options, let's set a filter on our eventLog logger.
	// We don't want to set this on the teelogger below because we want debug logs to always
	// go to debug.json.
	if opts.Debug {
		logger = level.NewFilter(logger, level.AllowDebug())
	} else {
		logger = level.NewFilter(logger, level.AllowInfo())
	}

	systemSlogger := multislogger.New(slog.NewJSONHandler(eventLogWriter, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	localSlogger := multislogger.New()

	// Create a local logger. This logs to a known path, and aims to help diagnostics
	if opts.RootDirectory != "" {
		ll := locallogger.NewKitLogger(filepath.Join(opts.RootDirectory, "debug.json"))
		logger = teelogger.New(logger, ll)

		localSloggerHandler := slog.NewJSONHandler(ll.Writer(), &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		})

		localSlogger.AddHandler(localSloggerHandler)

		// also write system logs to localSloggerHandler
		systemSlogger.AddHandler(localSloggerHandler)

		locallogger.CleanUpRenamedDebugLogs(opts.RootDirectory, logger)
	}

	// Use the FindNewest mechanism to delete old
	// updates. We do this here, as windows will pick up
	// the update in main, which does not delete.  Note
	// that this will likely produce non-fatal errors when
	// it tries to delete the running one.
	go func() {
		time.Sleep(15 * time.Second)
		_ = autoupdate.FindNewest(
			ctxlog.NewContext(context.TODO(), logger),
			os.Args[0],
			autoupdate.DeleteOldUpdates(),
		)
	}()

	// Confirm that service configuration is up-to-date
	checkServiceConfiguration(systemSlogger.Logger, opts)

	level.Info(logger).Log(
		"msg", "launching service",
		"version", version.Version().Version,
	)

	// Log panics from the windows service
	defer func() {
		if r := recover(); r != nil {
			level.Info(logger).Log(
				"msg", "panic occurred in windows service",
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

	if err := svc.Run(serviceName, &winSvc{
		logger:        logger,
		slogger:       localSlogger,
		systemSlogger: systemSlogger,
		opts:          opts,
	}); err != nil {
		// TODO The caller doesn't have the event log configured, so we
		// need to log here. this implies we need some deeper refactoring
		// of the logging
		level.Info(logger).Log(
			"msg", "error in service run",
			"err", err,
			"version", version.Version().Version,
		)
		time.Sleep(time.Second)
		return err
	}

	level.Debug(logger).Log("msg", "service exited", "version", version.Version().Version)
	time.Sleep(time.Second)

	return nil
}

func runWindowsSvcForeground(args []string) error {
	attachConsole()
	defer detachConsole()

	// Foreground mode is inherently a debug mode. So we start the
	// logger in debugging mode, instead of looking at opts.debug
	logger := logutil.NewCLILogger(true)
	level.Debug(logger).Log("msg", "foreground service start requested (debug mode)")

	// Use new logger to write logs to stdout
	systemSlogger := new(multislogger.MultiSlogger)
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
		os.Exit(1)
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
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	level.Debug(w.logger).Log("msg", "windows service starting")
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctx = ctxlog.NewContext(ctx, w.logger)

	go func() {
		err := runLauncher(ctx, cancel, w.slogger, w.systemSlogger, w.opts)
		if err != nil {
			level.Info(w.logger).Log("msg", "runLauncher exited", "err", err)
			level.Debug(w.logger).Log("msg", "runLauncher exited", "err", err, "stack", fmt.Sprintf("%+v", err))
			changes <- svc.Status{State: svc.Stopped, Accepts: cmdsAccepted}
			// Launcher is already shut down -- fully exit so that the service manager can restart the service
			os.Exit(1)
		}

		// If we get here, it means runLauncher returned nil. If we do
		// nothing, the service is left running, but with no
		// functionality. Instead, signal that as a stop to the service
		// manager, and exit. We rely on the service manager to restart.
		level.Info(w.logger).Log("msg", "runLauncher exited cleanly")
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
				level.Info(w.logger).Log("msg", "shutdown request received")
				changes <- svc.Status{State: svc.StopPending}
				cancel()
				time.Sleep(2 * time.Second) // give rungroups enough time to shut down
				changes <- svc.Status{State: svc.Stopped, Accepts: cmdsAccepted}
				return ssec, errno
			default:
				level.Info(w.logger).Log("msg", "unexpected change request", "change_request", fmt.Sprintf("%+v", c))
			}
		}
	}
}
