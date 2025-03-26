//go:build windows
// +build windows

package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/pkg/launcher"

	"golang.org/x/sys/windows"
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

	// currentVersionRegistryKeyFmt used to determine where the launcher installer info metadata will be,
	// we add or update the currentVersionKeyName alongside the existing keys from installation
	currentVersionRegistryKeyFmt = `Software\Kolide\Launcher\%s\%s`
	currentVersionKeyName        = `CurrentVersionNum`
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

	checkCurrentVersionMetadata(logger, opts.Identifier)

	checkRootDirACLs(logger, opts.RootDirectory)
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

// checkRecoveryActions checks if the recovery actions for the launcher service are set.
// sets if one or more of the recovery actions are not set.
// previously defined via wix ServicConfig Element (Util Extension) https://wixtoolset.org/docs/v3/xsd/util/serviceconfig/
func checkRecoveryActions(ctx context.Context, logger *slog.Logger, service *mgr.Service) {
	curRecoveryActions, err := service.RecoveryActions()
	if err != nil {
		logger.Log(context.TODO(), slog.LevelError,
			"querying for current RecoveryActions",
			"err", err,
		)

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

	// If the recovery actions are already set, we don't need to do anything
	if recoveryActionsAreSet(curRecoveryActions, recoveryActions) {
		return
	}

	if err := service.SetRecoveryActions(recoveryActions, 24*60*60); err != nil { // 24 hours
		logger.Log(ctx, slog.LevelError,
			"setting RecoveryActions",
			"err", err,
		)
	}
}

// recoveryActionsAreSet checks if the current recovery actions are set to the desired recovery actions
func recoveryActionsAreSet(curRecoveryActions, recoveryActions []mgr.RecoveryAction) bool {
	if curRecoveryActions == nil || len(curRecoveryActions) != len(recoveryActions) {
		return false
	}
	for i := range curRecoveryActions {
		if curRecoveryActions[i].Type != recoveryActions[i].Type {
			return false
		}
		if curRecoveryActions[i].Delay != recoveryActions[i].Delay {
			return false
		}
	}
	return true
}

// checkCurrentVersionMetadata ensures that we've set our currently running version number to
// the registry alongside the other installation metadata. this looks a little different than
// our other registry interactions (e.g. checkDelayedAutostart) because there are two different
// ways to set a value for a key: by setting its default (unnamed) value, or by setting a named
// value under the key path. we opt to set and get the default value for the full key path to
// maintain consistency with the pattern set by our installer info metadata.
// for additional details on the difference, see here https://devblogs.microsoft.com/oldnewthing/20080118-00/?p=23773
func checkCurrentVersionMetadata(logger *slog.Logger, identifier string) {
	versionKeyPath := fmt.Sprintf(currentVersionRegistryKeyFmt, identifier, currentVersionKeyName)
	launcherVersionKey, err := registry.OpenKey(registry.LOCAL_MACHINE, versionKeyPath, registry.ALL_ACCESS)
	// create the key if it doesn't already exist
	if err != nil && err.Error() == notFoundInRegistryError {
		launcherVersionKey, _, err = registry.CreateKey(registry.LOCAL_MACHINE, versionKeyPath, registry.ALL_ACCESS)
	}

	if err != nil {
		logger.Log(context.TODO(), slog.LevelError,
			"could not create or open new registry key",
			"key_path", versionKeyPath,
			"err", err,
		)

		return
	}

	defer launcherVersionKey.Close()

	// passing an empty name here to get the default value set for key
	currentVersionVal, _, getCurrentVersionErr := launcherVersionKey.GetIntegerValue("")
	expectedVersion := version.VersionNum()

	// take no action if we can read the current version and it matches expected
	if getCurrentVersionErr == nil && currentVersionVal == uint64(expectedVersion) {
		logger.Log(context.TODO(), slog.LevelDebug, "skipping writing current version info to registry",
			"expected_version", expectedVersion,
			"current_registry_version", currentVersionVal,
		)

		return
	}

	// if we can't read, or the version is out of date, set the expected version as the new value
	if err := launcherVersionKey.SetDWordValue("", uint32(expectedVersion)); err != nil {
		logger.Log(context.TODO(), slog.LevelError,
			"encountered error setting current version to registry",
			"err", err,
		)

		return
	}

	logger.Log(context.TODO(), slog.LevelInfo,
		"updated registry value for current version info",
		"updated_version", expectedVersion,
		"previous_registry_version", currentVersionVal,
	)
}

// checkRootDirACLs sets a security policy on the root directory to ensure that
// SYSTEM, administrators, and the directory owner have full access, but that regular
// users only have read/execute permission. errors are logged but not retried, as we will attempt this
// on every launcher startup
func checkRootDirACLs(logger *slog.Logger, rootDirectory string) {
	logger = logger.With("component", "checkRootDirACLs")

	if strings.TrimSpace(rootDirectory) == "" {
		logger.Log(context.TODO(), slog.LevelError,
			"unable to check directory permissions without root dir set, skipping",
			"root_dir", rootDirectory,
		)

		return
	}

	// Get all the SIDs we need for our permissions
	usersSID, err := windows.CreateWellKnownSid(windows.WinBuiltinUsersSid)
	if err != nil {
		logger.Log(context.TODO(), slog.LevelError,
			"failed getting builtin users SID",
			"err", err,
		)

		return
	}

	adminsSID, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		logger.Log(context.TODO(), slog.LevelError,
			"failed getting builtin admins SID",
			"err", err,
		)

		return
	}

	creatorOwnerSID, err := windows.CreateWellKnownSid(windows.WinCreatorOwnerSid)
	if err != nil {
		logger.Log(context.TODO(), slog.LevelError,
			"failed getting creator/owner SID",
			"err", err,
		)

		return
	}

	systemSID, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		logger.Log(context.TODO(), slog.LevelError,
			"failed getting SYSTEM SID",
			"err", err,
		)

		return
	}

	// We want to mirror the permissions set in Program Files:
	// SYSTEM, admin, and creator/owner have full control; users are allowed only read and execute.
	explicitAccessPolicies := []windows.EXPLICIT_ACCESS{
		{
			AccessPermissions: windows.GENERIC_ALL,
			AccessMode:        windows.SET_ACCESS,
			Inheritance:       windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT, // ensure access is inherited by sub folders
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_GROUP,
				TrusteeValue: windows.TrusteeValueFromSID(systemSID),
			},
		},
		{
			AccessPermissions: windows.GENERIC_ALL,
			AccessMode:        windows.SET_ACCESS,
			Inheritance:       windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT, // ensure access is inherited by sub folders
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_GROUP,
				TrusteeValue: windows.TrusteeValueFromSID(adminsSID),
			},
		},
		{
			AccessPermissions: windows.GENERIC_READ | windows.GENERIC_EXECUTE,
			AccessMode:        windows.SET_ACCESS,
			Inheritance:       windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT, // ensure access is inherited by sub folders
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_GROUP,
				TrusteeValue: windows.TrusteeValueFromSID(usersSID),
			},
		},
		{
			AccessPermissions: windows.GENERIC_ALL,
			AccessMode:        windows.SET_ACCESS,
			Inheritance:       windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT, // ensure access is inherited by sub folders
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_GROUP,
				TrusteeValue: windows.TrusteeValueFromSID(creatorOwnerSID),
			},
		},
	}

	// Overwrite the existing DACL
	newDACL, err := windows.ACLFromEntries(explicitAccessPolicies, nil)
	if err != nil {
		logger.Log(context.TODO(), slog.LevelError,
			"generating new DACL from access entries",
			"err", err,
		)

		return
	}

	// apply the new DACL to the root directory
	err = windows.SetNamedSecurityInfo(
		rootDirectory,
		windows.SE_FILE_OBJECT,
		// PROTECTED_DACL_SECURITY_INFORMATION here ensures we don't re-inherit the parent permissions
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, newDACL, nil,
	)

	if err != nil {
		logger.Log(context.TODO(), slog.LevelError,
			"setting named security info from new DACL",
			"err", err,
		)

		return
	}

	logger.Log(context.TODO(), slog.LevelInfo, "updated ACLs for root directory")
}
