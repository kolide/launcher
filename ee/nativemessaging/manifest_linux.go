//go:build linux

package nativemessaging

import "fmt"

// chromeManifestFileRegistrationLocations returns the filepaths where the native messaging manifest file should exist
// for this OS.
// See: https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging#native-messaging-host-location
func chromeManifestFileRegistrationLocations(hostName string) []string {
	return []string{
		fmt.Sprintf("/etc/opt/chrome/native-messaging-hosts/%s.json", hostName),
		fmt.Sprintf("/etc/opt/chrome_for_testing/native-messaging-hosts/%s.json", hostName),
		fmt.Sprintf("/etc/chromium/native-messaging-hosts/%s.json", hostName),
	}
}

// firefoxManifestFileRegistrationLocations returns the filepaths where the native messaging manifest file should exist
// for this OS.
// See: https://developer.mozilla.org/en-US/docs/Mozilla/Add-ons/WebExtensions/Native_manifests#manifest_location
func firefoxManifestFileRegistrationLocations(hostName string) []string {
	return []string{
		fmt.Sprintf("/usr/lib/mozilla/native-messaging-hosts/%s.json", hostName),
	}
}
