//go:build windows

package nativemessaging

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"
)

// manifestFileRegistrationLocations returns the registry key where we should write the path to the
// native messaging manifest file.
// See: https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging#native-messaging-host-location
func manifestFileRegistrationLocations(hostName string) []string {
	return []string{`SOFTWARE\Google\Chrome\NativeMessagingHosts\` + hostName}
}

// registerManifestFileLocation writes the manifest file location to the expected registry key
// at `registrationPath` so that Chrome knows where to find it.
// See: https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging#native-messaging-host-location
func registerManifestFileLocation(manifestFileLocation string, registrationPath string) error {
	manifestLocationKey, err := registry.OpenKey(registry.LOCAL_MACHINE, registrationPath, registry.ALL_ACCESS)

	// If the key doesn't exist, create it
	if err != nil && errors.Is(err, os.ErrNotExist) {
		manifestLocationKey, _, err = registry.CreateKey(registry.LOCAL_MACHINE, registrationPath, registry.ALL_ACCESS)
	}
	if err != nil {
		return fmt.Errorf("creating or opening registry key at %s: %w", registrationPath, err)
	}

	defer manifestLocationKey.Close()

	// Get the default value of the key by passing in an empty string.
	currentValue, _, err := manifestLocationKey.GetStringValue("")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("getting current default value of %s: %w", registrationPath, err)
	}

	// Already set correctly -- no need to edit
	if currentValue == manifestFileLocation {
		return nil
	}

	if err := manifestLocationKey.SetStringValue("", manifestFileLocation); err != nil {
		return fmt.Errorf("setting default value of %s to %s: %w", registrationPath, manifestFileLocation, err)
	}

	return nil
}

func deregisterManifestFileLocation(registrationPath string) error {
	if err := registry.DeleteKey(registry.LOCAL_MACHINE, registrationPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("deleting registry key at %s: %w", registrationPath, err)
	}

	return nil
}
