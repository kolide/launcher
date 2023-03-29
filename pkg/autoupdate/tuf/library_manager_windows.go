//go:build windows
// +build windows

package tuf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// executableLocation returns the path to the executable in `updateDirectory`.
func executableLocation(updateDirectory string, binary string) string {
	return filepath.Join(updateDirectory, fmt.Sprintf("%s.exe", binary))
}

// verifyExecutable cannot check executable bits on Windows because Windows
// does not have them, so it checks for the .exe file extension instead.
func verifyExecutable(fileInfo os.FileInfo) error {
	if !strings.HasSuffix(fileInfo.Name(), ".exe") {
		return fmt.Errorf("file %s is not executable", fileInfo.Name())
	}

	return nil
}
