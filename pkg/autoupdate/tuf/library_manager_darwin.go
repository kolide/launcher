//go:build darwin
// +build darwin

package tuf

import (
	"fmt"
	"os"
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

// verifyExecutable checks the executable bits on the given file.
func verifyExecutable(fileInfo os.FileInfo) error {
	if fileInfo.Mode()&0111 == 0 {
		return fmt.Errorf("file %s is not executable", fileInfo.Name())
	}

	return nil
}
