//go:build !windows
// +build !windows

package main

import (
	"errors"

	"github.com/kolide/launcher/pkg/log/multislogger"
)

func runWindowsSvc(_ *multislogger.MultiSlogger, _ []string) error {
	return errors.New("This isn't windows")
}

func runWindowsSvcForeground(_ *multislogger.MultiSlogger, _ []string) error {
	return errors.New("This isn't windows")
}
