//go:build !windows
// +build !windows

package restartservice

import (
	"errors"

	"github.com/kolide/launcher/pkg/log/multislogger"
)

func runRestartService(_ *multislogger.MultiSlogger, args []string) error {
	return errors.New("not implemented on non windows platforms")
}
