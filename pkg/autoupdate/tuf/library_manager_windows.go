//go:build windows
// +build windows

package tuf

import (
	"fmt"
	"path/filepath"
)

func executableLocation(updateDirectory string, binary string) string {
	return filepath.Join(updateDirectory, fmt.Sprintf("%s.exe", binary))
}
