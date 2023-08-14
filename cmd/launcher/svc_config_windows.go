//go:build windows
// +build windows

package main

import (
	"fmt"
	"path/filepath"

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

	customUrlKey = "ktest"
)

func registerCustomUrl(logger log.Logger, opts *launcher.Options) {
	// If this isn't a Kolide installation, do not register custom URL
	if opts.KolideServerURL != "k2device.kolide.com" && opts.KolideServerURL != "k2device-preprod.kolide.com" {
		return
	}

	// Get custom URL key
	customUrlEntry, openedExisting, err := registry.CreateKey(registry.CLASSES_ROOT, customUrlKey, registry.ALL_ACCESS)
	if err != nil {
		level.Error(logger).Log("msg", "could not create registry key", "key_name", customUrlKey, "err", err)
		return
	}
	// Close it once we're done
	defer func() {
		if err := customUrlEntry.Close(); err != nil {
			level.Error(logger).Log("msg", "could not close registry key", "key_name", customUrlKey, "err", err)
		}
	}()

	// Key already exists so we don't need to create configuration here
	if openedExisting {
		level.Error(logger).Log("msg", "key already exists, nothing to do", "key_name", customUrlKey)
		return
	}

	// Setup mirrors https://learn.microsoft.com/en-us/previous-versions/windows/internet-explorer/ie-developer/platform-apis/aa767914(v=vs.85)

	if err := customUrlEntry.SetStringValue("", fmt.Sprintf("URL:%s Protocol", customUrlKey)); err != nil {
		level.Error(logger).Log("msg", "cannot set (Default)", "err", err)
		return
	}

	if err := customUrlEntry.SetStringValue("URL Protocol", ""); err != nil {
		level.Error(logger).Log("msg", "cannot set URL Protocol key", "err", err)
		return
	}

	// Create DefaultIcon key under customUrlKey
	defaultIconEntry, _, err := registry.CreateKey(registry.CLASSES_ROOT, filepath.Join(customUrlKey, "DefaultIcon"), registry.ALL_ACCESS)
	if err != nil {
		level.Error(logger).Log("msg", "could not create registry key", "key_name", filepath.Join(customUrlKey, "DefaultIcon"), "err", err)
		return
	}
	// Close it once we're done
	defer func() {
		if err := defaultIconEntry.Close(); err != nil {
			level.Error(logger).Log("msg", "could not close registry key", "key_name", "DefaultIcon", "err", err)
		}
	}()

	if err := defaultIconEntry.SetStringValue("", `C:\Program Files\Kolide\Launcher-kolide-k2\bin\launcher.exe,1`); err != nil {
		level.Error(logger).Log("msg", "cannot set URL Protocol key", "err", err)
		return
	}

	// Create keys customUrlKey => shell => open => command
	shellPath := filepath.Join(customUrlKey, "shell")
	shellEntry, _, err := registry.CreateKey(registry.CLASSES_ROOT, shellPath, registry.ALL_ACCESS)
	if err != nil {
		level.Error(logger).Log("msg", "could not create registry key", "key_name", shellPath, "err", err)
		return
	}
	defer func() {
		if err := shellEntry.Close(); err != nil {
			level.Error(logger).Log("msg", "could not close registry key", "key_name", shellPath, "err", err)
		}
	}()

	openPath := filepath.Join(shellPath, "open")
	openEntry, _, err := registry.CreateKey(registry.CLASSES_ROOT, openPath, registry.ALL_ACCESS)
	if err != nil {
		level.Error(logger).Log("msg", "could not create registry key", "key_name", openPath, "err", err)
		return
	}
	defer func() {
		if err := openEntry.Close(); err != nil {
			level.Error(logger).Log("msg", "could not close registry key", "key_name", openPath, "err", err)
		}
	}()

	commandPath := filepath.Join(openPath, "command")
	commandEntry, _, err := registry.CreateKey(registry.CLASSES_ROOT, commandPath, registry.ALL_ACCESS)
	if err != nil {
		level.Error(logger).Log("msg", "could not create registry key", "key_name", commandPath, "err", err)
		return
	}
	defer func() {
		if err := commandEntry.Close(); err != nil {
			level.Error(logger).Log("msg", "could not close registry key", "key_name", commandPath, "err", err)
		}
	}()

	if err := commandEntry.SetStringValue("", `"C:\Program Files\Kolide\Launcher-kolide-k2\bin\launcher.exe" "%1"`); err != nil {
		level.Error(logger).Log("msg", "cannot set Default key", "err", err)
		return
	}
}

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
	checkDependOnService(launcherServiceKey, logger)
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

// checkDependOnService checks the current value of `DependOnService` (the list of services that must
// start before launcher can) and updates it if necessary.
func checkDependOnService(launcherServiceKey registry.Key, logger log.Logger) {
	serviceList, _, getServiceListErr := launcherServiceKey.GetStringsValue(dependOnServiceName)

	if getServiceListErr != nil {
		if getServiceListErr.Error() == notFoundInRegistryError {
			// `DependOnService` does not exist for this service yet -- we can safely set it to include the Dnscache service.
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
	for _, service := range serviceList {
		// Already included -- no need to update
		if service == dnscacheService {
			return
		}
	}

	// Set service to depend on Dnscache
	serviceList = append(serviceList, dnscacheService)
	if err := launcherServiceKey.SetStringsValue(dependOnServiceName, serviceList); err != nil {
		level.Error(logger).Log("msg", "could not set strings value for DependOnService", "err", err)
	}
}
