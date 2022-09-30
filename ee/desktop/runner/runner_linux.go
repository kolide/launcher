//go:build linux
// +build linux

package runner

import (
	"fmt"
	"os/exec"
)

func (r *DesktopUsersProcessesRunner) consoleUsers() ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func cmdAsUser(uid string, path string, args ...string) (*exec.Cmd, error) {
	return nil, fmt.Errorf("not implemented")
}
