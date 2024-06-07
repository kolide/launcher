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

var likelyRootDirPaths = []string{
	"C:\\ProgramData\\Kolide\\Launcher-kolide-k2\\data",
	"C:\\Program Files\\Kolide\\Launcher-kolide-k2\\data",
}

// runWindowsSvc starts launcher as a windows service. This will
// probably not behave correctly if you start it from the command line.
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
		rootDirectoryChanged := false
		optsRootDirectory := opts.RootDirectory
		// check for old root directories before creating DB in case we've stomped over with windows MSI install
		updatedRootDirectory := determineRootDirectory(systemSlogger.Logger, opts)
		if updatedRootDirectory != opts.RootDirectory {
			opts.RootDirectory = updatedRootDirectory
			// cache that we did this so we can log to debug.json when set up below
			rootDirectoryChanged = true
		}

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

		if rootDirectoryChanged {
			localSlogger.Log(context.TODO(), slog.LevelInfo,
				"old root directory contents detected, overriding opts.RootDirectory",
				"opts_root_directory", optsRootDirectory,
				"updated_root_directory", updatedRootDirectory,
			)
		}
	}

	// Confirm that service configuration is up-to-date
	checkServiceConfiguration(systemSlogger.Logger, opts)

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
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

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

// determineRootDirectory is used specifically for windows deployments to override the
// configured root directory if another one containing a launcher DB already exists
func determineRootDirectory(slogger *slog.Logger, opts *launcher.Options) string {
	optsRootDirectory := opts.RootDirectory
	// don't mess with the path if this installation isn't pointing to a kolide server URL
	if opts.KolideServerURL != "k2device.kolide.com" && opts.KolideServerURL != "k2device-preprod.kolide.com" {
		return optsRootDirectory
	}

	optsDBLocation := filepath.Join(optsRootDirectory, "launcher.db")
	dbExists, err := nonEmptyFileExists(optsDBLocation)
	// If we get an unknown error, back out from making any options changes. This is an
	// unlikely path but doesn't feel right updating the rootDirectory without knowing what's going
	// on here
	if err != nil {
		slogger.Log(context.TODO(), slog.LevelError,
			"encountered error checking for pre-existing launcher.db",
			"location", optsDBLocation,
			"err", err,
		)

		return optsRootDirectory
	}

	// database already exists in configured root directory, keep that
	if dbExists {
		return optsRootDirectory
	}

	// we know this is a fresh install with no launcher.db in the configured root directory,
	// check likely locations and return updated rootDirectory if found
	for _, path := range likelyRootDirPaths {
		if path == optsRootDirectory { // we already know this does not contain an enrolled DB
			continue
		}

		testingLocation := filepath.Join(path, "launcher.db")
		dbExists, err := nonEmptyFileExists(testingLocation)
		if err == nil && dbExists {
			return path
		}

		if err != nil {
			slogger.Log(context.TODO(), slog.LevelWarn,
				"encountered error checking non-configured locations for launcher.db",
				"opts_location", optsDBLocation,
				"tested_location", testingLocation,
				"err", err,
			)

			continue
		}
	}

	// if all else fails, return the originally configured rootDirectory -
	// this is expected for devices that are truly installing from MSI for the first time
	return optsRootDirectory
}

func nonEmptyFileExists(path string) (bool, error) {
	fileInfo, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return fileInfo.Size() > 0, nil
}
