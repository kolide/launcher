//go:build !windows
// +build !windows

package main

import (
	"errors"

	"github.com/kolide/launcher/pkg/log/multislogger"
)

func runQueryWindowsUpdates(systemMultiSlogger *multislogger.MultiSlogger, args []string) error {
	return errors.New("not implemented on non-windows platforms")
}
