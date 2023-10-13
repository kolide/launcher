//go:build darwin
// +build darwin

package tuf

import (
	"os"
	"path/filepath"
)

// executableLocation returns the path to the executable in `updateDirectory`.
// For launcher, and versions of osquery after 5.9.1, this means a path inside the app bundle.
func executableLocation(updateDirectory string, binary autoupdatableBinary) string {
	switch binary {
	case "launcher":
		return filepath.Join(updateDirectory, "Kolide.app", "Contents", "MacOS", string(binary))
	case "osqueryd":
		// Only return the path to the app bundle executable if it exists
		appBundleExecutable := filepath.Join(updateDirectory, "osquery.app", "Contents", "MacOS", string(binary))
		if _, err := os.Stat(appBundleExecutable); err == nil {
			return appBundleExecutable
		}

		// Older version of osquery
		return filepath.Join(updateDirectory, string(binary))
	default:
		return ""
	}
}
