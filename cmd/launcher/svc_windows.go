//go:build windows
// +build windows

package main

import (
	"context"
	"fmt"
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
	"github.com/kolide/launcher/pkg/log/teelogger"

	"golang.org/x/sys/windows/registry"
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

	opts, err := parseOptions(os.Args[2:])
	if err != nil {
		level.Info(logger).Log("msg", "Error parsing options", "err", err)
		os.Exit(1)
	}

	// Create a local logger. This logs to a known path, and aims to help diagnostics
	if opts.RootDirectory != "" {
		logger = teelogger.New(logger, locallogger.NewKitLogger(filepath.Join(opts.RootDirectory, "debug.json")))
		locallogger.CleanUpRenamedDebugLogs(opts.RootDirectory, logger)
	}

	// Now that we've parsed the options, let's set a filter on our logger
	if opts.Debug {
		logger = level.NewFilter(logger, level.AllowDebug())
	} else {
		logger = level.NewFilter(logger, level.AllowInfo())
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

	// For Kolide installations, confirm that service configuration is up-to-date
	if opts.KolideServerURL == "k2device.kolide.com" || opts.KolideServerURL == "k2device-preprod.kolide.com" {
		checkServiceConfiguration(logger)
	}

	level.Info(logger).Log(
		"msg", "launching service",
		"version", version.Version().Version,
	)

	// Log panics from the windows service
	defer func() {
		if r := recover(); r != nil {
			level.Info(logger).Log(
				"msg", "panic occurred",
				"err", err,
			)
			time.Sleep(time.Second)
		}
	}()

	if err := svc.Run(serviceName, &winSvc{logger: logger, opts: opts}); err != nil {
		// TODO The caller doesn't have the event log configured, so we
		// need to log here. this implies we need some deeper refactoring
		// of the logging
		level.Info(logger).Log(
			"msg", "Error in service run",
			"err", err,
			"version", version.Version().Version,
		)
		time.Sleep(time.Second)
		return err
	}

	level.Debug(logger).Log("msg", "Service exited", "version", version.Version().Version)
	time.Sleep(time.Second)

	return nil
}

func runWindowsSvcForeground(args []string) error {
	// Foreground mode is inherently a debug mode. So we start the
	// logger in debugging mode, instead of looking at opts.debug
	logger := logutil.NewCLILogger(true)
	level.Debug(logger).Log("msg", "foreground service start requested (debug mode)")

	opts, err := parseOptions(os.Args[2:])
	if err != nil {
		level.Info(logger).Log("err", err)
		os.Exit(1)
	}

	// set extra debug options
	opts.Debug = true
	opts.OsqueryVerbose = true

	run := debug.Run

	return run(serviceName, &winSvc{logger: logger, opts: opts})
}

type winSvc struct {
	logger log.Logger
	opts   *launcher.Options
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
		err := runLauncher(ctx, cancel, w.opts)
		if err != nil {
			level.Info(w.logger).Log("msg", "runLauncher exited", "err", err)
			level.Debug(w.logger).Log("msg", "runLauncher exited", "err", err, "stack", fmt.Sprintf("%+v", err))
			changes <- svc.Status{State: svc.Stopped, Accepts: cmdsAccepted}
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
				time.Sleep(100 * time.Millisecond)
				changes <- svc.Status{State: svc.Stopped, Accepts: cmdsAccepted}
				return
			default:
				level.Info(w.logger).Log("err", "unexpected control request", "control_request", c)
			}
		}
	}
}

const (
	launcherServiceRegistryKeyName = `SYSTEM\CurrentControlSet\Services\LauncherKolideK2Svc`

	// DelayedAutostart is type REG_DWORD, i.e. uint32. We want to turn off delayed autostart.
	delayedAutostartName            = `DelayedAutostart`
	delayedAutostartDisabled uint32 = 0

	// DependOnService is type REG_MULTI_SZ, i.e. a list of strings
	dependOnServiceName = `DependOnService`
	dnscacheService     = `Dnscache`
)

func checkServiceConfiguration(logger log.Logger) {
	// Get launcher service key
	launcherServiceKey, err := registry.OpenKey(registry.LOCAL_MACHINE, launcherServiceRegistryKeyName, registry.ALL_ACCESS)
	if err != nil {
		level.Error(logger).Log("msg", "could not open registry key", "key_name", launcherServiceRegistryKeyName, "err", err)
		return
	}

	// Close it once we're done
	defer func() {
		if err := launcherServiceKey.Close(); err != nil {
			level.Error(logger).Log("msg", "could not close registry key", "key_name", launcherServiceRegistryKeyName, "err", err)
		}
	}()

	// Check to see if we need to turn off delayed autostart
	currentDelayedAutostart, _, getDelayedAutostartErr := launcherServiceKey.GetIntegerValue(delayedAutostartName)
	if getDelayedAutostartErr == nil && currentDelayedAutostart != uint64(delayedAutostartDisabled) {
		// Turn off delayed autostart
		if err := launcherServiceKey.SetDWordValue(delayedAutostartName, delayedAutostartDisabled); err != nil {
			level.Error(logger).Log("msg", "could not set dword value for DelayedAutostart", "err", err)
		}
	}

	// Check to see if we need to update the service to depend on Dnscache
	serviceList, _, getServiceListErr := launcherServiceKey.GetStringsValue(dependOnServiceName)
	if getServiceListErr != nil {
		// If DependsOn isn't set at all yet, set it with the Dnscache service
		if getServiceListErr.Error() == "The system cannot find the file specified." {
			if err := launcherServiceKey.SetStringsValue(dependOnServiceName, []string{dnscacheService}); err != nil {
				level.Error(logger).Log("msg", "could not set strings value for DependOnService", "err", err)
			}
			return
		}

		// In any other case, if we can't get the current value, we don't want to proceed --
		// we don't want to wipe any current data from the list.
		return
	}

	foundDnscacheInList := false
	for _, service := range serviceList {
		if service == dnscacheService {
			foundDnscacheInList = true
			break
		}
	}

	if foundDnscacheInList {
		return
	}

	// Set service to depend on Dnscache
	serviceList = append(serviceList, dnscacheService)
	if err := launcherServiceKey.SetStringsValue(dependOnServiceName, serviceList); err != nil {
		level.Error(logger).Log("msg", "could not set strings value for DependOnService", "err", err)
	}
}
