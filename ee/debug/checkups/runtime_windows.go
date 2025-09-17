//go:build windows

package checkups

import (
	"errors"
)

func (c *runtimeCheckup) findDesktopProcessesWithLsof() ([]desktopProcessInfo, error) {
	// lsof is not available on Windows, system-wide discovery not supported
	return nil, errors.New("system-wide desktop process discovery not supported on Windows")
}

func (c *runtimeCheckup) getAuthTokenFromProcess(pid int) (string, error) {
	// Process environment reading via ps is not available on Windows
	return "", errors.New("process environment reading not supported on Windows")
}
