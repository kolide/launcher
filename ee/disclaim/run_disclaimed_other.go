//go:build !darwin
// +build !darwin

package disclaim

import (
	"errors"

	"github.com/kolide/launcher/pkg/log/multislogger"
)

func RunDisclaimed(_ *multislogger.MultiSlogger, args []string) error {
	return errors.New("run disclaimed is only implemented for macOS")
}
