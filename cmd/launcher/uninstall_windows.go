//go:build windows
// +build windows

package main

import (
	"context"
	"github.com/pkg/errors"
)

func removeLauncher(ctx context.Context, identifier string) error {
	// Uninstall is not implemented for Windows - users have to use add/remove programs themselves
	return errors.Errorf("Uninstall subcommand is not supported for Windows platforms.")
}
