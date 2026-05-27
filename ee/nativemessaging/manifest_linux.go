//go:build linux

package nativemessaging

import "fmt"

// manifestFilePaths returns the paths where the native messaging manifest file should exist
// for this OS.
// See: https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging#native-messaging-host-location
func manifestFilePaths(_ string) []string {
	return []string{
		fmt.Sprintf("/etc/opt/chrome/native-messaging-hosts/%s.json", nativeMessagingHostName),
		fmt.Sprintf("/etc/opt/chrome_for_testing/native-messaging-hosts/%s.json", nativeMessagingHostName),
		fmt.Sprintf("/etc/chromium/native-messaging-hosts/%s.json", nativeMessagingHostName),
	}
}

// registerManifestFileLocation is a no-op on non-Windows OSes because we write the manifest file
// to a well-known location already.
func registerManifestFileLocation(_ string, _ string) error {
	return nil
}
