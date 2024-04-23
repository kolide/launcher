//go:build windows
// +build windows

package main

import (
	"context"
	"errors"
)

func removeLauncher(ctx context.Context, identifier string) error {
	// Uninstall is not implemented for Windows - users have to use add/remove programs themselves
	return errors.New("Uninstall subcommand is not supported for Windows platforms.")
}
