//go:build windows

package listener

import "golang.org/x/sys/windows"

// hasPermissionsToRunTest return true if the current process has elevated permissions (administrator) --
// this is required to run tests on windows
func hasPermissionsToRunTest() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}
