//go:build windows
// +build windows

package runtime

import (
	"fmt"
)

func (r *DesktopUsersProcessesRunner) runConsoleUserDesktop() error {
	return fmt.Errorf("not implemented")
}

func processExists(pid int) bool {
	return false
}
