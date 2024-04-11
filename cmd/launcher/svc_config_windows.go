//go:build windows
// +build windows

package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/kolide/launcher/pkg/launcher"

	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	launcherServiceName            = `LauncherKolideK2Svc`
	launcherServiceRegistryKeyName = `SYSTEM\CurrentControlSet\Services\LauncherKolideK2Svc`
	launcherAccountName            = `Kolide` //?

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
func checkRestartActions(serviceManager *mgr.Mgr, slogger *slog.Logger) {
	launcherService, err := serviceManager.OpenService(launcherServiceName)
	if err != nil {
		slogger.Log(context.TODO(), slog.LevelError,
			"opening the launcher restart service from control manager",
			"err", err,
		)

		return
	}

	defer launcherService.Close()

	curFlag, err := launcherService.RecoveryActionsOnNonCrashFailures()
	if err != nil {
		slogger.Log(context.TODO(), slog.LevelError,
			"querying for current RecoveryActionsOnNonCrashFailures flag",
			"err", err,
		)

		return
	}

	if curFlag { // nothing to do, the flag was already set correctly
		return
	}

	if err = launcherService.SetRecoveryActionsOnNonCrashFailures(true); err != nil {
		slogger.Log(context.TODO(), slog.LevelError,
			"setting RecoveryActionsOnNonCrashFailures flag",
			"err", err,
		)

		return
	}

	slogger.Log(context.TODO(), slog.LevelInfo, "successfully set RecoveryActionsOnNonCrashFailures flag")
}

func checkRestartService(serviceManager *mgr.Mgr, slogger *slog.Logger) {
	slogger = slogger.With("service", launcherRestartServiceName)
	// first check if we've already installed the service
	_, err := serviceManager.OpenService(launcherRestartServiceName)
	if err == nil {
		// commented out code here enables re-installation on restart, may need to be wired into updates
		// if err = restartService.Delete(); err != nil {
		// 	slogger.Log(context.TODO(), slog.LevelError,
		// 		"failed to delete existing service",
		// 		"err", err,
		// 	)

		// 	return
		// }

		// if err = restartService.Close(); err != nil {
		// 	slogger.Log(context.TODO(), slog.LevelError,
		// 		"failed to close existing service",
		// 		"err", err,
		// 	)
		// }

		// TODO swap this out for code above so we aren't constantly re-installing
		slogger.Log(context.TODO(), slog.LevelDebug,
			"service already exists, skipping installation",
			"service", launcherRestartServiceName,
		)

		return
	}

	currentExe, err := os.Executable()
	if err != nil {
		slogger.Log(context.TODO(), slog.LevelError,
			"installing launcher restart service, unable to collect current executable path",
			"service", launcherRestartServiceName,
			"err", err,
		)
	}

	svcMgrConf := mgr.Config{
		DisplayName:      launcherRestartServiceName,
		StartType:        mgr.StartAutomatic,
		ErrorControl:     mgr.ErrorNormal,
		DelayedAutoStart: true, // seems safest to wait until after launcher proper has attempted to start
	}

	restartService, err := serviceManager.CreateService(
		launcherRestartServiceName,
		currentExe,
		svcMgrConf,
		"restart-service",
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
