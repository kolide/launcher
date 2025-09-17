//go:build windows

package checkups

import (
	"errors"
)

func (c *runtimeCheckup) findDesktopProcessesWithLsof() ([]desktopProcessInfo, error) {
	// lsof is not available on Windows, system-wide discovery not supported
	return nil, errors.New("system-wide desktop process discovery not supported on Windows")
}
