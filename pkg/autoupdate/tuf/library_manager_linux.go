//go:build linux
// +build linux

package tuf

import "path/filepath"

func executableLocation(updateDirectory string, binary string) string {
	return filepath.Join(updateDirectory, binary)
}
