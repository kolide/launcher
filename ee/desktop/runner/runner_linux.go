//go:build linux
// +build linux

package runner

import (
	"fmt"
	"os"
)

func (r *DesktopUsersProcessesRunner) consoleUsers() ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func runAsUser(uid string, envVars []string, path string, args ...string) (*os.Process, error) {
	return nil, fmt.Errorf("not implemented")
}
