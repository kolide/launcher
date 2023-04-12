//go:build linux
// +build linux

package tuf

import (
	"path/filepath"
)

// executableLocation returns the path to the executable in `updateDirectory`.
func executableLocation(updateDirectory string, binary autoupdatableBinary) string {
	return filepath.Join(updateDirectory, string(binary))
}
