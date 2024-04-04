//go:build !darwin
// +build !darwin

package main

import (
	"errors"

	"github.com/kolide/launcher/pkg/log/multislogger"
)

func runSecureEnclave(_ *multislogger.MultiSlogger, args []string) error {
	return errors.New("not implemented on non darwin platforms")
}
