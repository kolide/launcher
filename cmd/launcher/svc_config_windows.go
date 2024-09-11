//go:build windows
// +build windows

package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kolide/launcher/pkg/launcher"

	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	launcherServiceRegistryKeyNameFmt = `SYSTEM\CurrentControlSet\Services\%s`

	// DelayedAutostart is type REG_DWORD, i.e. uint32. We want to turn off delayed autostart.
	delayedAutostartName            = `DelayedAutostart`
	delayedAutostartDisabled uint32 = 0

	// DependOnService is type REG_MULTI_SZ, i.e. a list of strings
	dependOnServiceName = `DependOnService`
	dnscacheService     = `Dnscache`

	notFoundInRegistryError = "The system cannot find the file specified."
)

func checkServiceConfiguration(logger *slog.Logger, opts *launcher.Options) {
	// If this isn't a Kolide installation, do not update the configuration
	if !launcher.IsKolideHostedServerURL(opts.KolideServerURL) {
		return
	}

	// get the service name to generate the service key
	launcherServiceName := launcher.ServiceName(opts.Identifier)
	launcherServiceRegistryKeyName := fmt.Sprintf(launcherServiceRegistryKeyNameFmt, launcherServiceName)

	// Get launcher service key
	launcherServiceKey, err := registry.OpenKey(registry.LOCAL_MACHINE, launcherServiceRegistryKeyName, registry.ALL_ACCESS)
	if err != nil {
		logger.Log(context.TODO(), slog.LevelError,
			"could not open registry key",
			"key_name", launcherServiceRegistryKeyName,
			"err", err,
		)

		return
	}

	// Close it once we're done
	defer func() {
		if err := launcherServiceKey.Close(); err != nil {
			logger.Log(context.TODO(), slog.LevelError,
				"could not close registry key",
				"key_name", launcherServiceRegistryKeyName,
				"err", err,
			)
		}
	}()

	// Check to see if we need to turn off delayed autostart
	checkDelayedAutostart(launcherServiceKey, logger)

	// Check to see if we need to update the service to depend on Dnscache
	checkDependOnService(launcherServiceKey, logger)

	sman, err := mgr.Connect()
	if err != nil {
		logger.Log(context.TODO(), slog.LevelError,
			"connecting to service control manager",
			"err", err,
		)

		return
	}

	defer sman.Disconnect()

	launcherService, err := sman.OpenService(launcherServiceName)
	if err != nil {
		logger.Log(context.TODO(), slog.LevelError,
			"opening the launcher service from control manager",
			"err", err,
		)

		return
	}

	defer launcherService.Close()

	checkRestartActions(logger, launcherService)

	checkRecoveryActions(context.TODO(), logger, launcherService)
}

// checkDelayedAutostart checks the current value of `DelayedAutostart` (whether to wait ~2 minutes
// before starting the launcher service) and updates it if necessary.
func checkDelayedAutostart(launcherServiceKey registry.Key, logger *slog.Logger) {
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
		logger.Log(context.TODO(), slog.LevelError,
			"could not turn off DelayedAutostart",
			"err", err,
		)
	}
}

// checkDependOnService checks the current value of `DependOnService` (the list of services that must
// start before launcher can) and updates it if necessary.
func checkDependOnService(launcherServiceKey registry.Key, logger *slog.Logger) {
	serviceList, _, getServiceListErr := launcherServiceKey.GetStringsValue(dependOnServiceName)

	if getServiceListErr != nil {
		if getServiceListErr.Error() == notFoundInRegistryError {
			// `DependOnService` does not exist for this service yet -- we can safely set it to include the Dnscache service.
			if err := launcherServiceKey.SetStringsValue(dependOnServiceName, []string{dnscacheService}); err != nil {
				logger.Log(context.TODO(), slog.LevelError,
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
		logger.Log(context.TODO(), slog.LevelError,
			"could not set strings value for DependOnService",
			"err", err,
		)
	}
}

// checkRestartActions checks the current value of our `SERVICE_FAILURE_ACTIONS_FLAG` and
// sets it to true if required. See https://learn.microsoft.com/en-us/windows/win32/api/winsvc/ns-winsvc-service_failure_actions_flag
// if we choose to implement restart backoff, that logic must be added here (it is not exposed via wix). See the "Windows Service Manager"
// doc in Notion for additional details on configurability
func checkRestartActions(logger *slog.Logger, service *mgr.Service) {
	curFlag, err := service.RecoveryActionsOnNonCrashFailures()
	if err != nil {
		logger.Log(context.TODO(), slog.LevelError,
			"querying for current RecoveryActionsOnNonCrashFailures flag",
			"err", err,
		)

		return
	}

	if curFlag { // nothing to do, the flag was already set correctly
		return
	}

	if err = service.SetRecoveryActionsOnNonCrashFailures(true); err != nil {
		logger.Log(context.TODO(), slog.LevelError,
			"setting RecoveryActionsOnNonCrashFailures flag",
			"err", err,
		)

		return
	}

	logger.Log(context.TODO(), slog.LevelInfo, "successfully set RecoveryActionsOnNonCrashFailures flag")
}

// setRecoveryActions sets the recovery actions for the launcher service.
// previously defined via wix ServicConfig Element (Util Extension) https://wixtoolset.org/docs/v3/xsd/util/serviceconfig/
func checkRecoveryActions(ctx context.Context, logger *slog.Logger, service *mgr.Service) {
	curFlag, err := service.RecoveryActionsOnNonCrashFailures()

	if err != nil {
		logger.Log(context.TODO(), slog.LevelError,
			"querying for current RecoveryActionsOnNonCrashFailures flag",
			"err", err,
		)

		return
	}

	if curFlag { // nothing to do, the flag was already set correctly
		return
	}
	recoveryActions := []mgr.RecoveryAction{
		{
			// first failure
			Type:  mgr.ServiceRestart,
			Delay: 5 * time.Second,
		},
		{
			// second failure
			Type:  mgr.ServiceRestart,
			Delay: 5 * time.Second,
		},
		{
			// subsequent failures
			Type:  mgr.ServiceRestart,
			Delay: 5 * time.Second,
		},
	}

	if err := service.SetRecoveryActions(recoveryActions, 24*60*60); err != nil { // 24 hours
		logger.Log(ctx, slog.LevelError,
			"setting RecoveryActions",
			"err", err,
		)
	}
}
