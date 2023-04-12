//go:build darwin
// +build darwin

package tuf

import (
	"path/filepath"
)

// executableLocation returns the path to the executable in `updateDirectory`.
// For launcher, this means a path inside the app bundle.
func executableLocation(updateDirectory string, binary autoupdatableBinary) string {
	switch binary {
	case "launcher":
		return filepath.Join(updateDirectory, "Kolide.app", "Contents", "MacOS", string(binary))
	case "osqueryd":
		return filepath.Join(updateDirectory, string(binary))
	default:
		return ""
	}
}
