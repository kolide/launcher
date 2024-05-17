//go:build windows
// +build windows

package watchdog

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"time"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	agentsqlite "github.com/kolide/launcher/ee/agent/storage/sqlite"
	"github.com/kolide/launcher/ee/agent/types"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	launcherWatchdogServiceName string = `LauncherKolideWatchdogSvc`
	launcherServiceName         string = `LauncherKolideK2Svc`

	serviceDoesNotExistError string = "The specified service does not exist as an installed service."
)

// WatchdogController is responsible for:
//  1. adding/enabling and disabling/removing the watchdog service according to the agent flag
//  2. publishing any watchdog_logs written out by the watchdog service
//
// This controller is intended for use by the main launcher service invocation
type WatchdogController struct {
	slogger      *slog.Logger
	knapsack     types.Knapsack
	interrupt    chan struct{}
	interrupted  bool
	logPublisher types.LogStore
}

func NewController(ctx context.Context, k types.Knapsack) (*WatchdogController, error) {
	// set up the log publisher, if watchdog is enabled we will need to pull those logs from sqlite periodically
	logPublisher, err := agentsqlite.OpenRW(ctx, k.RootDirectory(), agentsqlite.WatchdogLogStore)
	if err != nil {
		return nil, fmt.Errorf("opening log db in %s: %w", k.RootDirectory(), err)
	}

	return &WatchdogController{
		slogger:      k.Slogger().With("component", "watchdog_controller"),
		knapsack:     k,
		interrupt:    make(chan struct{}, 1),
		logPublisher: logPublisher,
	}, nil
}

func (wc *WatchdogController) FlagsChanged(flagKeys ...keys.FlagKey) {
	if slices.Contains(flagKeys, keys.LauncherWatchdogEnabled) {
		wc.ServiceEnabledChanged(wc.knapsack.LauncherWatchdogEnabled())
	}
}

// Run starts a log publication routine. The purpose of this is to
// pull logs out of the sqlite database and write them to debug.json so we can
// use all of the existing log publication and cleanup logic while maintaining a single writer
func (wc *WatchdogController) Run() error {
	ctx := context.TODO()
	ticker := time.NewTicker(time.Minute * 5)
	defer ticker.Stop()

	for {
		wc.Once(ctx)

		select {
		case <-ticker.C:
			continue
		case <-wc.interrupt:
			wc.slogger.Log(ctx, slog.LevelDebug,
				"interrupt received, exiting execute loop",
			)
			return nil
		}
	}
}

func (wc *WatchdogController) Once(ctx context.Context) {
	// do nothing if watchdog is not enabled
	if !wc.knapsack.LauncherWatchdogEnabled() {
		return
	}

	logsToDelete := make([]any, 0)

	if err := wc.logPublisher.ForEach(func(rowid, timestamp int64, v []byte) error {
		logRecord := make(map[string]any)
		if err := json.Unmarshal(v, &logRecord); err != nil {
			wc.slogger.Log(ctx, slog.LevelError, "failed to unmarshal sqlite log", "log", string(v))
			logsToDelete = append(logsToDelete, rowid)
			// log the issue but don't return an error, we want to keep processing whatever we can
			return nil
		}

		logArgs := make([]slog.Attr, len(logRecord))
		for k, v := range logRecord {
			logArgs = append(logArgs, slog.Any(k, v))
		}

		// re-issue the log, this time with the debug.json writer
		// pulling out the existing log and re-adding all attributes like this will overwrite
		// the automatic timestamp creation, as well as the msg and level set below
		wc.slogger.LogAttrs(ctx, slog.LevelInfo, "", logArgs...)
		logsToDelete = append(logsToDelete, rowid)

		return nil
	}); err != nil {
		wc.slogger.Log(ctx, slog.LevelError, "iterating sqlite logs", "err", err)
		return
	}

	wc.slogger.Log(ctx, slog.LevelInfo, "collected logs for deletion", "rowids", logsToDelete)

	if err := wc.logPublisher.DeleteRows(logsToDelete...); err != nil {
		wc.slogger.Log(ctx, slog.LevelError, "cleaning up published sqlite logs", "err", err)
	}
}

func (wc *WatchdogController) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if wc.interrupted {
		return
	}

	wc.logPublisher.Close()
	wc.interrupted = true
	wc.interrupt <- struct{}{}
}

func (wc *WatchdogController) ServiceEnabledChanged(enabled bool) {
	ctx := context.TODO()

	serviceManager, err := mgr.Connect()
	if err != nil {
		wc.slogger.Log(ctx, slog.LevelError,
			"connecting to service control manager",
			"err", err,
		)

		return
	}

	defer serviceManager.Disconnect()

	if !enabled {
		err := removeService(serviceManager, launcherWatchdogServiceName)
		if err != nil && err.Error() != serviceDoesNotExistError {
			wc.slogger.Log(ctx, slog.LevelWarn,
				"encountered error removing watchdog service",
				"err", err,
			)
		}

		return
	}

	// we're enabling the watchdog - first check if we've already installed the service
	// there are three potential paths here:
	// 1. service did not previously exist, proceed with clean installation
	existingService, err := serviceManager.OpenService(launcherWatchdogServiceName)
	if err != nil && err.Error() == serviceDoesNotExistError {
		if err = wc.installService(serviceManager); err != nil {
			wc.slogger.Log(ctx, slog.LevelError,
				"encountered error installing watchdog service",
				"err", err,
			)
		}

		return
	}

	// 2. we are unable to check the current status of the service,
	// this is the least likely option and there's nothing we can do here so log and return
	if err != nil {
		wc.slogger.Log(ctx, slog.LevelWarn,
			"encountered error checking for watchdog service, unable to proceed with enabling",
			"err", err,
		)

		return
	}

	// 3. The watchdog service already exists on this device. Here we just restart it to ensure it is
	// running on the latest launcher code
	defer existingService.Close()
	if err = wc.restartService(existingService); err != nil {
		wc.slogger.Log(ctx, slog.LevelError,
			"failure attempting to restart watchdog service",
			"err", err,
		)
	}
}

func (wc *WatchdogController) installService(serviceManager *mgr.Mgr) error {
	ctx := context.TODO()
	currentExe, err := os.Executable()
	if err != nil {
		wc.slogger.Log(ctx, slog.LevelError,
			"installing launcher watchdog, unable to collect current executable path",
			"err", err,
		)
	}

	svcMgrConf := mgr.Config{
		DisplayName:  launcherWatchdogServiceName,
		Description:  "The Kolide Launcher Watchdog Service",
		StartType:    mgr.StartAutomatic,
		ErrorControl: mgr.ErrorNormal,
		// no reason to rush start for this service, we should wait until after
		// the main launcher service has attempted to start anyway
		DelayedAutoStart: true,
	}

	serviceArgs := []string{"watchdog"}
	// add any original service arguments from the main launcher service invocation (currently running)
	// this is likely just a pointer to the launcher.flags file but we want to ensure that the watchdog service
	// has insight into the same options for early service configuration, logging, etc.
	serviceArgs = append(serviceArgs, os.Args[2:]...)

	restartService, err := serviceManager.CreateService(
		launcherWatchdogServiceName,
		currentExe,
		svcMgrConf,
		serviceArgs...,
	)

	if err != nil { // no point moving forward if we can't create the service
		return err
	}

	defer restartService.Close()

	recoveryActions := []mgr.RecoveryAction{
		{
			Type:  mgr.ServiceRestart,
			Delay: 5 * time.Second,
		},
	}

	if err = restartService.SetRecoveryActions(recoveryActions, 10800); err != nil {
		wc.slogger.Log(ctx, slog.LevelWarn,
			"unable to set recovery actions for service installation, proceeding",
			"err", err,
		)
	}

	if err = restartService.SetRecoveryActionsOnNonCrashFailures(true); err != nil {
		wc.slogger.Log(ctx, slog.LevelWarn,
			"unable to set RecoveryActionsOnNonCrashFailures flag, proceeding",
			"err", err,
		)
	}

	if err = restartService.Start(); err != nil {
		wc.slogger.Log(ctx, slog.LevelWarn,
			"unable to start launcher restart service",
			"err", err,
		)
	}

	wc.slogger.Log(ctx, slog.LevelInfo, "completed watchdog service installation")

	return nil
}

// removeService utilizes the passed serviceManager to remove the existing service
// after looking up the handle from serviceName
func removeService(serviceManager *mgr.Mgr, serviceName string) error {
	existingService, err := serviceManager.OpenService(serviceName)
	if err != nil {
		return err
	}

	defer existingService.Close()

	if err = existingService.Delete(); err != nil {
		return err
	}

	return nil
}

func (wc *WatchdogController) restartService(service *mgr.Service) error {
	status, err := service.Control(svc.Stop)
	if err != nil {
		wc.slogger.Log(context.TODO(), slog.LevelWarn,
			"error stopping service",
			"err", err,
		)

		// always attempt to start the service regardless, if the service was already
		// stopped it will still err on the control (stop) call above
		return service.Start()
	}

	timeout := time.Now().Add(10 * time.Second)
	for status.State != svc.Stopped {
		if timeout.Before(time.Now()) {
			return fmt.Errorf("timeout waiting for %s service to stop", service.Name)
		}

		time.Sleep(500 * time.Millisecond)
		status, err = service.Query()
		if err != nil {
			return fmt.Errorf("could not retrieve service status: %w", err)
		}
	}

	return service.Start()
}