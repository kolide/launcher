//go:build linux

package nativemessaging

import "fmt"

// manifestFileRegistrationLocations returns the filepaths where the native messaging manifest file should exist
// for this OS.
// See: https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging#native-messaging-host-location
func manifestFileRegistrationLocations() []string {
	return []string{
		fmt.Sprintf("/etc/opt/chrome/native-messaging-hosts/%s.json", nativeMessagingHostName),
		fmt.Sprintf("/etc/opt/chrome_for_testing/native-messaging-hosts/%s.json", nativeMessagingHostName),
		fmt.Sprintf("/etc/chromium/native-messaging-hosts/%s.json", nativeMessagingHostName),
	}
}
