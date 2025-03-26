//go:build !windows
// +build !windows

package watchdog

import (
	"errors"

	"github.com/kolide/launcher/pkg/log/multislogger"
)

func RunWatchdogTask(_ *multislogger.MultiSlogger, args []string) error {
	return errors.New("not implemented on non windows platforms")
}
