// +build windows

package table

import "path"

// zerotierCli returns the path to the CLI executable.
func zerotierCli(args ...string) []string {
	cmd := []string{
		path.Join("C:", "ProgramData", "ZeroTier", "One", "zerotier-one_x64.exe"),
		"-q",
	}

	return append(cmd, args...)
}
