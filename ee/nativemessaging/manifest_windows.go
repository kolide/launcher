//go:build windows

package nativemessaging

import (
	"fmt"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

const (
	notYetCreatedRegistryErrStr = "The system cannot find the file specified."
)

// manifestFilePaths returns the paths where the native messaging manifest file should exist
// for this OS. On Windows, the location can be anywhere (we specify the location via a
// well-known registry key), so we write it to the root directory.
// See: https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging#native-messaging-host-location
func manifestFilePaths(rootDir string) []string {
	return []string{filepath.Join(rootDir, "nmh-manifest.json")}
}

// registerManifestFileLocation writes the manifest file location to the expected registry key,
// so that Chrome knows where to find it.
// See: https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging#native-messaging-host-location
func registerManifestFileLocation(manifestFileLocation string, registryKeyPath string) error {
	manifestLocationKey, err := registry.OpenKey(registry.LOCAL_MACHINE, registryKeyPath, registry.ALL_ACCESS)

	// If the key doesn't exist, create it
	if err != nil && err.Error() == notYetCreatedRegistryErrStr {
		manifestLocationKey, _, err = registry.CreateKey(registry.LOCAL_MACHINE, registryKeyPath, registry.ALL_ACCESS)
	}
	if err != nil {
		return fmt.Errorf("creating or opening registry key at %s: %w", registryKeyPath, err)
	}

	defer manifestLocationKey.Close()

	// Get the default value of the key by passing in an empty string.
	currentValue, _, err := manifestLocationKey.GetStringValue("")
	if err != nil && err.Error() != notYetCreatedRegistryErrStr {
		return fmt.Errorf("getting current default value of %s: %w", registryKeyPath, err)
	}

	// Already set correctly -- no need to edit
	if currentValue == manifestFileLocation {
		return nil
	}

	if err := manifestLocationKey.SetStringValue("", manifestFileLocation); err != nil {
		return fmt.Errorf("setting default value of %s to %s: %w", registryKeyPath, manifestFileLocation, err)
	}

	return nil
}
