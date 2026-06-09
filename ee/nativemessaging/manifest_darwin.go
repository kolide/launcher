//go:build darwin

package nativemessaging

import "fmt"

// manifestFileRegistrationLocations returns the filepaths where the native messaging manifest file should exist
// for this OS.
// See: https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging#native-messaging-host-location
func manifestFileRegistrationLocations() []string {
	return []string{
		fmt.Sprintf("/Library/Google/Chrome/NativeMessagingHosts/%s.json", nativeMessagingHostName),
		fmt.Sprintf("/Library/Google/ChromeForTesting/NativeMessagingHosts/%s.json", nativeMessagingHostName),
		fmt.Sprintf("/Library/Application Support/Chromium/NativeMessagingHosts/%s.json", nativeMessagingHostName),
	}
}
