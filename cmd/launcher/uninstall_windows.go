//go:build windows
// +build windows

package main

import (
	"context"
	"os/exec"
)

func removeLauncher(ctx context.Context, identifier string) error {
	// Launch the Windows Settings app using the ms-settings: URI scheme
	// https://learn.microsoft.com/en-us/windows/uwp/launch-resume/launch-settings-app#apps
	cmd := exec.CommandContext(ctx, "start", []string{"ms-settings:appsfeatures"}...)
	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}
