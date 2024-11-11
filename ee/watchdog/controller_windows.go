//go:build windows
// +build windows

package watchdog

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
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
	taskDateFormat              string = "2006-01-02T15:04:05"
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

	// TODO generate task name based on identifier
	taskName := "LauncherKolideK2WakerTask"

	if !enabled {
		if err := RemoveWatchdogTask(taskName); err != nil {
			wc.slogger.Log(ctx, slog.LevelWarn,
				"encountered error removing watchdog task",
				"err", err,
			)

			return
		}

		wc.slogger.Log(ctx, slog.LevelInfo, "removed watchdog task")

		return
	}

	// we're enabling the watchdog task- we can safely always reinstall our latest version here
	if err := InstallWatchdogTask(taskName); err != nil {
		wc.slogger.Log(ctx, slog.LevelError,
			"encountered error installing watchdog task",
			"err", err,
		)
	}

	wc.slogger.Log(ctx, slog.LevelInfo, "completed watchdog scheduled task installation")
}

// TODO make this interpolate identifier in path generation
func getExecutablePath() (string, error) {
	defaultBinDir := launcher.DefaultPath(launcher.BinDirectory)
	defaultLauncherLocation := filepath.Join(defaultBinDir, "launcher.exe")
	// do some basic sanity checking to prevent installation from a bad path
	_, err := os.Stat(defaultLauncherLocation)
	if err != nil {
		return "", err
	}

	return defaultLauncherLocation, nil
}

func InstallWatchdogTask(taskName string) error {
	// Initialize COM
	ole.CoInitialize(0)
	defer ole.CoUninitialize()

	// Create a Task Scheduler object
	schedService, err := oleutil.CreateObject("Schedule.Service")
	if err != nil {
		return fmt.Errorf("creating schedule service object: %w", err)
	}
	defer schedService.Release()

	// Get the Task Scheduler service interface
	scheduler, err := schedService.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return err
	}
	defer scheduler.Release()

	// Connect to the local machine
	_, err = oleutil.CallMethod(scheduler, "Connect")
	if err != nil {
		return fmt.Errorf("failed to connect to Task Scheduler: %w", err)
	}

	// Get the root task folder
	rootFolderVar, err := oleutil.CallMethod(scheduler, "GetFolder", `\`)
	if err != nil {
		return fmt.Errorf("failed to get root folder: %w", err)
	}

	rootFolder := rootFolderVar.ToIDispatch()
	defer rootFolder.Release()

	// Create a new task definition
	taskDefinitionTemplate, err := oleutil.CallMethod(scheduler, "NewTask", 0)
	if err != nil {
		return fmt.Errorf("failed to create new task definition: %w", err)
	}

	taskDefinition := taskDefinitionTemplate.ToIDispatch()
	defer taskDefinition.Release()

	installationDate := time.Now().Format(taskDateFormat)

	// Get task registration info
	regInfoProp, err := oleutil.GetProperty(taskDefinition, "RegistrationInfo")
	if err != nil {
		return fmt.Errorf("getting registration info property: %w", err)
	}
	regInfo := regInfoProp.ToIDispatch()
	defer regInfo.Release()

	if _, err = oleutil.PutProperty(regInfo, "Description", "Kolide agent waker"); err != nil {
		return fmt.Errorf("setting reginfo description: %w", err)
	}

	if _, err := oleutil.PutProperty(regInfo, "Author", "Kolide, Inc."); err != nil {
		return fmt.Errorf("setting reginfo author: %w", err)
	}

	if _, err := oleutil.PutProperty(regInfo, "Date", installationDate); err != nil {
		return fmt.Errorf("setting reginfo date: %w", err)
	}

	principalProp, err := oleutil.GetProperty(taskDefinition, "Principal")
	if err != nil {
		return fmt.Errorf("getting principal property: %w", err)
	}

	principal := principalProp.ToIDispatch()
	defer principal.Release()

	// see all principal settings here https://learn.microsoft.com/en-us/windows/win32/api/taskschd/nn-taskschd-iprincipal
	// 1=TASK_RUNLEVEL_HIGHEST
	if _, err := oleutil.PutProperty(principal, "RunLevel", uint(1)); err != nil {
		return fmt.Errorf("setting run level: %w", err)
	}

	// Get the task settings
	settingsProp, err := oleutil.GetProperty(taskDefinition, "Settings")
	if err != nil {
		return fmt.Errorf("getting settings property: %w", err)
	}

	settings := settingsProp.ToIDispatch()
	defer settings.Release()

	// TODO check all of these errors
	// see all available task settings here https://learn.microsoft.com/en-us/windows/win32/api/taskschd/nn-taskschd-itasksettings
	// task will be enabled on creation
	if _, err = oleutil.PutProperty(settings, "Enabled", true); err != nil {
		return fmt.Errorf("setting enabled flag: %w", err)
	}

	// start the task at any time after its scheduled time has passed
	if _, err = oleutil.PutProperty(settings, "StartWhenAvailable", true); err != nil {
		return fmt.Errorf("setting StartWhenAvailable flag: %w", err)
	}

	// task will be started even if the computer is running on batteries
	if _, err = oleutil.PutProperty(settings, "DisallowStartIfOnBatteries", false); err != nil {
		return fmt.Errorf("setting DisallowStartIfOnBatteries flag: %w", err)
	}

	// task will be continue even if the computer changes power source to battery
	if _, err = oleutil.PutProperty(settings, "StopIfGoingOnBatteries", false); err != nil {
		return fmt.Errorf("setting StopIfGoingOnBatteries flag: %w", err)
	}

	// see compatibility options here https://learn.microsoft.com/en-us/windows/win32/api/taskschd/ne-taskschd-task_compatibility
	// 2=TASK_COMPATIBILITY_V2 - recommended unless you need to support Windows XP, Windows Server 2003, or Windows 2000
	if _, err = oleutil.PutProperty(settings, "Compatibility", uint(2)); err != nil {
		return fmt.Errorf("setting Compatibility flag: %w", err)
	}

	idleSettingsProp, err := oleutil.GetProperty(settings, "IdleSettings")
	if err != nil {
		return fmt.Errorf("getting idle settings property: %w", err)
	}

	idleSettings := idleSettingsProp.ToIDispatch()
	defer idleSettings.Release()

	// see idle settings here https://learn.microsoft.com/en-us/windows/win32/taskschd/taskschedulerschema-idlesettings-settingstype-element
	// do not stop the task if an idle condition ends before the task is completed
	if _, err = oleutil.PutProperty(idleSettings, "StopOnIdleEnd", false); err != nil {
		return fmt.Errorf("setting StopOnIdleEnd idlesetting: %w", err)
	}

	// Define the trigger
	triggersProp, err := oleutil.GetProperty(taskDefinition, "Triggers")
	if err != nil {
		return fmt.Errorf("getting triggers property: %w", err)
	}

	triggers := triggersProp.ToIDispatch()
	defer triggers.Release()
	// see trigger types here https://learn.microsoft.com/en-us/windows/win32/api/taskschd/ne-taskschd-task_trigger_type2
	createTriggerResp, err := oleutil.CallMethod(triggers, "Create", uint(0)) // 0=TASK_TRIGGER_EVENT
	if err != nil {
		log.Fatalf("encountered error creating trigger: %s", err.Error())
	}

	trigger := createTriggerResp.ToIDispatch()
	defer trigger.Release()

	if _, err = oleutil.PutProperty(trigger, "ExecutionTimeLimit", "PT1M"); err != nil {
		return fmt.Errorf("setting execution time limit property")
	}

	// found the guid here https://github.com/capnspacehook/taskmaster/blob/1629df7c85e96aab410af7f1747ba264d3276505/fill.go#L168
	eventTrigger, err := trigger.QueryInterface(ole.NewGUID("{d45b0167-9653-4eef-b94f-0732ca7af251}"))
	if err != nil {
		return fmt.Errorf("getting trigger interface: %w", err)
	}
	defer eventTrigger.Release()

	eventSubscriptionTemplate := `
<QueryList>
	<Query Path="System">
		<Select Path="System">*[System[Provider[@Name='Microsoft-Windows-Kernel-Power'] and (EventID=%d or EventID=%d)]]</Select>
		<Select Path="System">*[System[Provider[@Name='Microsoft-Windows-Power-Troubleshooter'] and (EventID=%d)]]</Select>
	</Query>
</QueryList>
`
	eventSubscription := fmt.Sprintf(eventSubscriptionTemplate, 507, 107, 1)

	if _, err = oleutil.PutProperty(eventTrigger, "Subscription", eventSubscription); err != nil {
		return fmt.Errorf("setting subscription property: %w", err)
	}
	// see details for how this string is created here: https://learn.microsoft.com/en-us/windows/win32/taskschd/eventtrigger-delay
	// PT1M here means 1 minute
	if _, err = oleutil.PutProperty(eventTrigger, "Delay", "PT1M"); err != nil {
		return fmt.Errorf("setting event trigger delay: %w", err)
	}

	// add another trigger, this one time based- repeat every 30 minutes
	createTimeTriggerResp, err := oleutil.CallMethod(triggers, "Create", uint(1)) // 1=TASK_TRIGGER_TIME
	if err != nil {
		return fmt.Errorf("error creating time trigger object: %w", err)
	}

	timeTrigger := createTimeTriggerResp.ToIDispatch()
	defer timeTrigger.Release()

	if _, err := oleutil.PutProperty(timeTrigger, "Enabled", true); err != nil {
		return fmt.Errorf("setting time trigger enabled: %w", err)
	}

	// set the execution timeout, PT1M=1 minute
	if _, err := oleutil.PutProperty(timeTrigger, "ExecutionTimeLimit", "PT1M"); err != nil {
		return fmt.Errorf("setting time trigger execution time limit: %w", err)
	}

	if _, err = oleutil.PutProperty(timeTrigger, "StartBoundary", installationDate); err != nil {
		return fmt.Errorf("setting time trigger start boundary: %w", err)
	}

	repetitionObj, err := oleutil.GetProperty(timeTrigger, "Repetition")
	if err != nil {
		return fmt.Errorf("getting time trigger repetition property: %w", err)
	}

	repetition := repetitionObj.ToIDispatch()
	defer repetition.Release()

	// set the repetition interval. PT30M=every 30 minutes
	if _, err = oleutil.PutProperty(repetition, "Interval", "PT30M"); err != nil {
		return fmt.Errorf("setting time trigger interval: %w", err)
	}

	// Define the action
	actionsProp, err := oleutil.GetProperty(taskDefinition, "Actions")
	if err != nil {
		return fmt.Errorf("getting actions property: %w", err)
	}

	actions := actionsProp.ToIDispatch()
	defer actions.Release()

	// see action types here https://learn.microsoft.com/en-us/windows/win32/api/taskschd/ne-taskschd-task_action_type
	// 0=TASK_ACTION_EXEC
	execActionTemplate, err := oleutil.CallMethod(actions, "Create", uint(0))
	if err != nil {
		return fmt.Errorf("creating event action: %w", err)
	}

	execAction := execActionTemplate.ToIDispatch()
	defer execAction.Release()

	installedExePath, err := getExecutablePath()
	if err != nil {
		return fmt.Errorf("determining watchdog executable path: %w", err)
	}

	serviceArgs := []string{"watchdog"}
	// add any original service arguments from the main launcher service invocation (currently running)
	// this is likely just a pointer to the launcher.flags file but we want to ensure that the watchdog
	// has insight into the same options for service name determination based on identifier, logging setup, etc.
	serviceArgs = append(serviceArgs, os.Args[2:]...)

	if _, err = oleutil.PutProperty(execAction, "Path", installedExePath); err != nil {
		return fmt.Errorf("setting action path: %w", err)
	}

	argString := strings.Join(serviceArgs, " ")
	if _, err = oleutil.PutProperty(execAction, "Arguments", argString); err != nil {
		return fmt.Errorf("setting action arguments: %w", err)
	}

	// Register the task
	_, err = oleutil.CallMethod(rootFolder, "RegisterTaskDefinition",
		taskName,       // Task name
		taskDefinition, // Task definition
		uint(6),        // Flags: 6=TASK_CREATE_OR_UPDATE see https://learn.microsoft.com/en-us/windows/win32/api/taskschd/ne-taskschd-task_creation
		"SYSTEM",       // User: run as system
		nil,            // password (nil for the current user, we expect this installed from SYSTEM)
		uint(5),        // 5=TASK_LOGON_SERVICE_ACCOUNT see https://learn.microsoft.com/en-us/windows/win32/api/taskschd/ne-taskschd-task_logon_type
		nil,            // SDDL (security descriptor definition language string, nil for our purposes here)
	)

	if err != nil {
		return fmt.Errorf("registering task definition: %w", err)
	}

	return nil
}

func RemoveWatchdogTask(taskName string) error {
	// Initialize COM
	ole.CoInitialize(0)
	defer ole.CoUninitialize()

	// Create a Task Scheduler object
	schedService, err := oleutil.CreateObject("Schedule.Service")
	if err != nil {
		return fmt.Errorf("creating schedule service object: %w", err)
	}
	defer schedService.Release()

	// Get the Task Scheduler service interface
	scheduler, err := schedService.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return err
	}
	defer scheduler.Release()

	// Connect to the local machine
	_, err = oleutil.CallMethod(scheduler, "Connect")
	if err != nil {
		return fmt.Errorf("failed to connect to Task Scheduler: %w", err)
	}

	// Get the root task folder
	rootFolderVar, err := oleutil.CallMethod(scheduler, "GetFolder", `\`)
	if err != nil {
		return fmt.Errorf("failed to get root folder: %w", err)
	}

	rootFolder := rootFolderVar.ToIDispatch()
	defer rootFolder.Release()

	// Delete the task
	_, err = oleutil.CallMethod(rootFolder, "DeleteTask", taskName, 0)
	if err != nil {
		return fmt.Errorf("failed to delete task %s: %w", taskName, err)
	}

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
