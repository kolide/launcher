//go:build darwin

package nativemessaging

import "fmt"

// chromeManifestFileRegistrationLocations returns the filepaths where the native messaging manifest file should exist
// for this OS.
// See: https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging#native-messaging-host-location
func chromeManifestFileRegistrationLocations(hostName string) []string {
	return []string{
		fmt.Sprintf("/Library/Google/Chrome/NativeMessagingHosts/%s.json", hostName),
		fmt.Sprintf("/Library/Google/ChromeForTesting/NativeMessagingHosts/%s.json", hostName),
		fmt.Sprintf("/Library/Application Support/Chromium/NativeMessagingHosts/%s.json", hostName),
	}
}

// firefoxManifestFileRegistrationLocations returns the filepaths where the native messaging manifest file should exist
// for this OS.
// See: https://developer.mozilla.org/en-US/docs/Mozilla/Add-ons/WebExtensions/Native_manifests#manifest_location
func firefoxManifestFileRegistrationLocations(hostName string) []string {
	return []string{
		fmt.Sprintf("/Library/Application Support/Mozilla/NativeMessagingHosts/%s.json", hostName),
	}
}
