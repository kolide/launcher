//go:build windows
// +build windows

package tuf

import (
	"fmt"
	"path/filepath"
)

// executableLocation returns the path to the executable in `updateDirectory`.
func executableLocation(updateDirectory string, binary autoupdatableBinary) string {
	return filepath.Join(updateDirectory, fmt.Sprintf("%s.exe", binary))
}
