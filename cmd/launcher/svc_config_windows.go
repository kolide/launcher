//go:build windows
// +build windows

package main

import (
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/launcher"

	"golang.org/x/sys/windows/registry"
)

const (
	launcherServiceRegistryKeyName = `SYSTEM\CurrentControlSet\Services\LauncherKolideK2Svc`

	// DelayedAutostart is type REG_DWORD, i.e. uint32. We want to turn off delayed autostart.
	delayedAutostartName            = `DelayedAutostart`
	delayedAutostartDisabled uint32 = 0

	// DependOnService is type REG_MULTI_SZ, i.e. a list of strings
	dependOnServiceName = `DependOnService`
	dnscacheService     = `Dnscache`

	notFoundInRegistryError = "The system cannot find the file specified."
)

func checkServiceConfiguration(logger log.Logger, opts *launcher.Options) {
	// If this isn't a Kolide installation, do not update the configuration
	if opts.KolideServerURL != "k2device.kolide.com" && opts.KolideServerURL != "k2device-preprod.kolide.com" {
		return
	}

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
	checkDelayedAutostart(launcherServiceKey, logger)

	// Check to see if we need to update the service to depend on Dnscache
	checkDependsOn(launcherServiceKey, logger)
}

// checkDelayedAutostart checks the current value of `DelayedAutostart` (whether to wait ~2 minutes
// before starting the launcher service) and updates it if necessary.
func checkDelayedAutostart(launcherServiceKey registry.Key, logger log.Logger) {
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
		level.Error(logger).Log("msg", "could not turn off DelayedAutostart", "err", err)
	}
}

// checkDependsOn checks the current value of `DependsOn` (the list of services that must start before
// launcher can) and updates it if necessary.
func checkDependsOn(launcherServiceKey registry.Key, logger log.Logger) {
	serviceList, _, getServiceListErr := launcherServiceKey.GetStringsValue(dependOnServiceName)

	if getServiceListErr != nil {
		if getServiceListErr.Error() == notFoundInRegistryError {
			// `DependsOn` does not exist for this service yet -- we can safely set it to include the Dnscache service.
			if err := launcherServiceKey.SetStringsValue(dependOnServiceName, []string{dnscacheService}); err != nil {
				level.Error(logger).Log("msg", "could not set strings value for DependOnService", "err", err)
			}
			return
		}

		// In any other case, if we can't get the current value, we don't want to proceed --
		// we don't want to wipe any current data from the list.
		return
	}

	// Check whether the service configuration already includes Dnscache in the list of services it depends on
	foundDnscacheInList := false
	for _, service := range serviceList {
		if service == dnscacheService {
			foundDnscacheInList = true
			break
		}
	}

	// Dnscache is already listed in services that launcher depends on -- no need to update
	if foundDnscacheInList {
		return
	}

	// Set service to depend on Dnscache
	serviceList = append(serviceList, dnscacheService)
	if err := launcherServiceKey.SetStringsValue(dependOnServiceName, serviceList); err != nil {
		level.Error(logger).Log("msg", "could not set strings value for DependOnService", "err", err)
	}
}
