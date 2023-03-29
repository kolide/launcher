//go:build darwin
// +build darwin

package tuf

import (
	"path/filepath"
)

// executableLocation returns the path to the executable in `updateDirectory`.
// For launcher, this means a path inside the app bundle.
func executableLocation(updateDirectory string, binary string) string {
	switch binary {
	case "launcher":
		return filepath.Join(updateDirectory, "Kolide.app", "Contents", "MacOS", binary)
	case "osqueryd":
		return filepath.Join(updateDirectory, binary)
	default:
		return ""
	}
}
