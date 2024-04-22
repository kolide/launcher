//go:build windows
// +build windows

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/kolide/launcher/pkg/launcher"

	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	launcherServiceName            = `LauncherKolideK2Svc`
	launcherServiceRegistryKeyName = `SYSTEM\CurrentControlSet\Services\LauncherKolideK2Svc`

	// DelayedAutostart is type REG_DWORD, i.e. uint32. We want to turn off delayed autostart.
	delayedAutostartName            = `DelayedAutostart`
	delayedAutostartDisabled uint32 = 0

	// DependOnService is type REG_MULTI_SZ, i.e. a list of strings
	dependOnServiceName = `DependOnService`
	dnscacheService     = `Dnscache`

	notFoundInRegistryError = "The system cannot find the file specified."
)

func checkServiceConfiguration(slogger *slog.Logger, opts *launcher.Options) {
	// If this isn't a Kolide installation, do not update the configuration
	if opts.KolideServerURL != "k2device.kolide.com" && opts.KolideServerURL != "k2device-preprod.kolide.com" {
		return
	}

	// Get launcher service key
	launcherServiceKey, err := registry.OpenKey(registry.LOCAL_MACHINE, launcherServiceRegistryKeyName, registry.ALL_ACCESS)
	if err != nil {
		slogger.Log(context.TODO(), slog.LevelError,
			"could not open registry key",
			"key_name", launcherServiceRegistryKeyName,
			"err", err,
		)

		return
	}

	// Close it once we're done
	defer func() {
		if err := launcherServiceKey.Close(); err != nil {
			slogger.Log(context.TODO(), slog.LevelError,
				"could not close registry key",
				"key_name", launcherServiceRegistryKeyName,
				"err", err,
			)
		}
	}()

	// Check to see if we need to turn off delayed autostart
	checkDelayedAutostart(launcherServiceKey, slogger)

	// Check to see if we need to update the service to depend on Dnscache
	checkDependOnService(launcherServiceKey, slogger)

	sman, err := mgr.Connect()
	if err != nil {
		slogger.Log(context.TODO(), slog.LevelError,
			"connecting to service control manager",
			"err", err,
		)

		return
	}

	defer sman.Disconnect()

	checkRestartActions(sman, slogger)
	checkRestartService(sman, slogger)
}

// checkDelayedAutostart checks the current value of `DelayedAutostart` (whether to wait ~2 minutes
// before starting the launcher service) and updates it if necessary.
func checkDelayedAutostart(launcherServiceKey registry.Key, slogger *slog.Logger) {
	currentDelayedAutostart, _, getDelayedAutostartErr := launcherServiceKey.GetIntegerValue(delayedAutostartName)

	// Can't determine current value, don't update
	if getDelayedAutostartErr != nil {
		return
	}

	// Delayed autostart is already disabled
	if currentDelayedAutostart == uint64(delayedAutostartDisabled) {
		return
	}

	// Turn off delayed autostart
	if err := launcherServiceKey.SetDWordValue(delayedAutostartName, delayedAutostartDisabled); err != nil {
		slogger.Log(context.TODO(), slog.LevelError,
			"could not turn off DelayedAutostart",
			"err", err,
		)
	}
}

// checkDependOnService checks the current value of `DependOnService` (the list of services that must
// start before launcher can) and updates it if necessary.
func checkDependOnService(launcherServiceKey registry.Key, slogger *slog.Logger) {
	serviceList, _, getServiceListErr := launcherServiceKey.GetStringsValue(dependOnServiceName)

	if getServiceListErr != nil {
		if getServiceListErr.Error() == notFoundInRegistryError {
			// `DependOnService` does not exist for this service yet -- we can safely set it to include the Dnscache service.
			if err := launcherServiceKey.SetStringsValue(dependOnServiceName, []string{dnscacheService}); err != nil {
				slogger.Log(context.TODO(), slog.LevelError,
					"could not set strings value for DependOnService",
					"err", err,
				)
			}
			return
		}

		// In any other case, if we can't get the current value, we don't want to proceed --
		// we don't want to wipe any current data from the list.
		return
	}

	// Check whether the service configuration already includes Dnscache in the list of services it depends on
	for _, service := range serviceList {
		// Already included -- no need to update
		if service == dnscacheService {
			return
		}
	}

	// Set service to depend on Dnscache
	serviceList = append(serviceList, dnscacheService)
	if err := launcherServiceKey.SetStringsValue(dependOnServiceName, serviceList); err != nil {
		slogger.Log(context.TODO(), slog.LevelError,
			"could not set strings value for DependOnService",
			"err", err,
		)
	}
}

// checkRestartActions checks the current value of our `SERVICE_FAILURE_ACTIONS_FLAG` and
// sets it to true if required. See https://learn.microsoft.com/en-us/windows/win32/api/winsvc/ns-winsvc-service_failure_actions_flag
// if we choose to implement restart backoff, that logic must be added here (it is not exposed via wix). See the "Windows Service Manager"
// doc in Notion for additional details on configurability
func checkRestartActions(serviceManager *mgr.Mgr, slogger *slog.Logger) {
	logCtx := context.TODO()
	launcherService, err := serviceManager.OpenService(launcherServiceName)
	if err != nil {
		slogger.Log(logCtx, slog.LevelError,
			"opening the launcher restart service from control manager",
			"err", err,
		)

		return
	}

	defer launcherService.Close()

	curFlag, err := launcherService.RecoveryActionsOnNonCrashFailures()
	if err != nil {
		slogger.Log(logCtx, slog.LevelError,
			"querying for current RecoveryActionsOnNonCrashFailures flag",
			"err", err,
		)

		return
	}

	if curFlag { // nothing to do, the flag was already set correctly
		return
	}

	if err = launcherService.SetRecoveryActionsOnNonCrashFailures(true); err != nil {
		slogger.Log(logCtx, slog.LevelError,
			"setting RecoveryActionsOnNonCrashFailures flag",
			"err", err,
		)

		return
	}

	slogger.Log(logCtx, slog.LevelInfo, "successfully set RecoveryActionsOnNonCrashFailures flag")
}

// serviceExists utilizes the service manager to determine if there is already a registered
// service for serviceName. It handles closing the service handle (if present)
func serviceExists(serviceManager *mgr.Mgr, serviceName string) bool { // nolint:unused
	existingService, err := serviceManager.OpenService(serviceName)
	if err == nil {
		existingService.Close()
		return true
	}

	return false
}

func restartService(service *mgr.Service) error {
	status, err := service.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("stopping %s service: %w", service.Name, err)
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

func checkRestartService(serviceManager *mgr.Mgr, slogger *slog.Logger) {
	logCtx := context.TODO()
	slogger = slogger.With("target_service", launcherRestartServiceName)
	// first check if we've already installed the service
	existingService, err := serviceManager.OpenService(launcherRestartServiceName)
	if err == nil {
		// if the service already exists, just restart it to ensure it's running from the latest launcher update.
		// If this fails, log the error but move on, this service is not worth tying up the main launcher startup.
		if err = restartService(existingService); err != nil {
			slogger.Log(logCtx, slog.LevelError,
				"failure attempting to restart service",
				"err", err,
			)
		}

		existingService.Close()
		return
	}

	// if we need to make any changes to the initial configuration of this service,
	// (e.g. all logic below here), we will likely need to rework this method to re-install,
	// rather than restart the service. This may make more sense to do as part of the upgrade process
	// instead of standard start up flow
	currentExe, err := os.Executable()
	if err != nil {
		slogger.Log(logCtx, slog.LevelError,
			"installing launcher restart service, unable to collect current executable path",
			"err", err,
		)
	}

	svcMgrConf := mgr.Config{
		DisplayName:  launcherRestartServiceName,
		Description:  "The Kolide Launcher Restart Service",
		StartType:    mgr.StartAutomatic,
		ErrorControl: mgr.ErrorNormal,
		// no reason to rush start for this service, we should wait until after
		// launcher proper has attempted to start anyway
		DelayedAutoStart: true,
	}

	serviceArgs := []string{"restart-service"}
	// add any original service arguments from the launcher proper invocation (currently running)
	serviceArgs = append(serviceArgs, os.Args[2:]...)

	restartService, err := serviceManager.CreateService(
		launcherRestartServiceName,
		currentExe,
		svcMgrConf,
		serviceArgs...,
	)

	if err != nil {
		slogger.Log(context.TODO(), slog.LevelWarn,
			"unable to create launcher restart service",
			"err", err,
		)

		return
	}

	defer restartService.Close()

	recoveryActions := []mgr.RecoveryAction{
		{
			Type:  mgr.ServiceRestart,
			Delay: 5 * time.Second,
		},
	}

	if err = restartService.SetRecoveryActions(recoveryActions, 10800); err != nil {
		slogger.Log(context.TODO(), slog.LevelWarn,
			"unable to set recovery actions for service installation, proceeding",
			"service", launcherRestartServiceName,
			"err", err,
		)
	}

	if err = restartService.SetRecoveryActionsOnNonCrashFailures(true); err != nil {
		slogger.Log(context.TODO(), slog.LevelWarn,
			"unable to set RecoveryActionsOnNonCrashFailures flag, proceeding",
			"service", launcherRestartServiceName,
			"err", err,
		)
	}

	if err = restartService.Start(); err != nil {
		slogger.Log(context.TODO(), slog.LevelWarn,
			"unable to start launcher restart service",
			"service", launcherRestartServiceName,
			"err", err,
		)
	}
}
