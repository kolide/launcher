//go:build darwin
// +build darwin

package tuf

import "path/filepath"

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
