//go:build linux
// +build linux

package tuf

import (
	"fmt"
	"os"
	"path/filepath"
)

// executableLocation returns the path to the executable in `updateDirectory`.
func executableLocation(updateDirectory string, binary string) string {
	return filepath.Join(updateDirectory, binary)
}

// verifyExecutable checks the executable bits on the given file.
func verifyExecutable(fileInfo os.FileInfo) error {
	if fileInfo.Mode()&0111 == 0 {
		return fmt.Errorf("file %s is not executable", fileInfo.Name())
	}

	return nil
}
