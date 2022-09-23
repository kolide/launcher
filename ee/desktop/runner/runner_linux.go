//go:build linux
// +build linux

package runner

import (
	"fmt"
)

func (r *DesktopUsersProcessesRunner) consoleUsers() ([]string, error) {
	return fmt.Errorf("not implemented")
}

func runAsUser(uid string, envVars []string, path string, args ...string) (*os.Process, error) {
	return fmt.Errorf("not implemented")
}
