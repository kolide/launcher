//go:build windows
// +build windows

package runtime

import (
	"testing"

	"golang.org/x/sys/windows"
)

func requirePgidMatch(_ *testing.T, _ int) {}

// hasPermissionsToRunTest return true if the current process has elevated permissions (administrator),
// this is required to run tests on windows
func hasPermissionsToRunTest() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}
