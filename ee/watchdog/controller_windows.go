//go:build windows
// +build windows

package watchdog

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	agentsqlite "github.com/kolide/launcher/ee/agent/storage/sqlite"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/launcher"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	launcherWatchdogServiceName string = `LauncherKolideWatchdogSvc`
	launcherServiceName         string = `LauncherKolideK2Svc`

	serviceDoesNotExistError string = "The specified service does not exist as an installed service."
)

// WatchdogController is responsible for:
//  1. adding/enabling and disabling/removing the watchdog task according to the agent flag
//  2. publishing any watchdog_logs written out by the watchdog task
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
		wc.publishLogs(ctx)

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

func (wc *WatchdogController) publishLogs(ctx context.Context) {
	// note that there is a small window here where there could be pending logs before watchdog is disabled -
	// there is no harm in leaving them and we could recover these with the original timestamps if we ever needed.
	// to avoid endlessly re-processing empty logs while we are disabled, we accept this possibility and exit early here
	if !wc.knapsack.LauncherWatchdogEnabled() {
		return
	}

	// we don't install watchdog for non-prod deployments, so we should also skip log publication
	if !launcher.IsKolideHostedServerURL(wc.knapsack.KolideServerURL()) {
		return
	}

	logsToDelete := make([]any, 0)

	if err := wc.logPublisher.ForEach(func(rowid, timestamp int64, v []byte) error {
		logRecord := make(map[string]any)
		logsToDelete = append(logsToDelete, rowid)

		if err := json.Unmarshal(v, &logRecord); err != nil {
			wc.slogger.Log(ctx, slog.LevelError,
				"failed to unmarshal sqlite log",
				"log", string(v),
				"err", err,
			)

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

		return nil
	}); err != nil {
		wc.slogger.Log(ctx, slog.LevelError, "iterating sqlite logs", "err", err)
		return
	}

	if len(logsToDelete) == 0 { // nothing else to do
		return
	}

	wc.slogger.Log(ctx, slog.LevelDebug, "collected logs for deletion", "rowids", logsToDelete)

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
	// we don't alter watchdog installation (install or remove) if this is a non-prod deployment
	if !launcher.IsKolideHostedServerURL(wc.knapsack.KolideServerURL()) {
		wc.slogger.Log(ctx, slog.LevelDebug,
			"skipping ServiceEnabledChanged for launcher watchdog in non-prod environment",
			"server_url", wc.knapsack.KolideServerURL(),
			"enabled", enabled,
		)

		return
	}

	// we also don't alter watchdog installation if we're running without elevated permissions
	if !windows.GetCurrentProcessToken().IsElevated() {
		wc.slogger.Log(ctx, slog.LevelDebug,
			"skipping ServiceEnabledChanged for launcher watchdog running without elevated permissions",
			"enabled", enabled,
		)

		return
	}

	var serviceManager *mgr.Mgr
	var err error

	if err := backoff.WaitFor(func() error {
		serviceManager, err = mgr.Connect()
		if err != nil {
			return fmt.Errorf("err connecting to service control manager: %w", err)
		}

		return nil
	}, 5*time.Second, 500*time.Millisecond); err != nil {
		wc.slogger.Log(ctx, slog.LevelError,
			"timed out connecting to service control manager",
			"err", err,
		)

		return
	}

	defer serviceManager.Disconnect()

	if !enabled {
		// TODO replace with remove task
		err := RemoveService(serviceManager)
		if err != nil {
			if err.Error() == serviceDoesNotExistError {
				return
			}

			wc.slogger.Log(ctx, slog.LevelWarn,
				"encountered error removing watchdog service",
				"err", err,
			)

			return
		}

		wc.slogger.Log(ctx, slog.LevelInfo, "removed watchdog service")

		return
	}

	// we're enabling the watchdog task- we can safely always reinstall our latest version here
	if err = wc.installWatchdogTask(); err != nil {
		wc.slogger.Log(ctx, slog.LevelError,
			"encountered error installing watchdog task",
			"err", err,
		)
	}
}

func (wc *WatchdogController) getExecutablePath() (string, error) {
	defaultBinDir := launcher.DefaultPath(launcher.BinDirectory)
	defaultLauncherLocation := filepath.Join(defaultBinDir, "launcher.exe")
	// do some basic sanity checking to prevent installation from a bad path
	_, err := os.Stat(defaultLauncherLocation)
	if err != nil {
		return "", err
	}

	return defaultLauncherLocation, nil
}

func (wc *WatchdogController) installWatchdogTask() error {
	ctx := context.TODO()
	installedExePath, err := wc.getExecutablePath()
	if err != nil {
		return fmt.Errorf("determining watchdog executable path: %w", err)
	}

	serviceArgs := []string{"watchdog"}
	// add any original service arguments from the main launcher service invocation (currently running)
	// this is likely just a pointer to the launcher.flags file but we want to ensure that the watchdog
	// has insight into the same options for early service configuration, logging, etc.
	serviceArgs = append(serviceArgs, os.Args[2:]...)

	// TODO add task installation logic

	wc.slogger.Log(ctx, slog.LevelInfo, "completed watchdog scheduled task installation")

	return nil
}

// RemoveService utilizes the passed serviceManager to remove any existing watchdog service if it exists
func RemoveService(serviceManager *mgr.Mgr) error {
	existingService, err := serviceManager.OpenService(launcherWatchdogServiceName)
	if err != nil {
		return err
	}

	defer existingService.Close()

	// attempt to stop the service first, we don't care if this fails because we are going to
	// remove the service next anyway (the removal happens faster if stopped first, but will
	// happen eventually regardless)
	existingService.Control(svc.Stop)

	if err := backoff.WaitFor(func() error {
		if err = existingService.Delete(); err != nil {
			return err
		}

		return nil
	}, 3*time.Second, 500*time.Millisecond); err != nil {
		return fmt.Errorf("timed out attempting service deletion: %w", err)
	}

	return nil
}
