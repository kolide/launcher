//go:build linux
// +build linux

package runtime

import (
	"fmt"
)

func (r *DesktopUsersProcessesRunner) runDesktopNative() error {
	return fmt.Errorf("not implemented")
}

func processExists(pid int) bool {
	return false
}
