//go:build !darwin
// +build !darwin

package allowedcmd

import (
	"errors"

	"github.com/kolide/launcher/pkg/log/multislogger"
)

func Run(_ *multislogger.MultiSlogger, args []string) error {
	return errors.New("run disclaimed is only implemented for macOS")
}
