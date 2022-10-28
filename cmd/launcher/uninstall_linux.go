//go:build linux
// +build linux

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

func removeLauncher(ctx context.Context, identifier string) error {
	if identifier == "" {
		identifier = "kolide-k2"
	}

	serviceName := fmt.Sprintf("launcher.%s", identifier)
	packageName := fmt.Sprintf("launcher-%s", identifier)

	// Stop launcher service
	cmd := exec.CommandContext(ctx, "systemctl", []string{"stop", serviceName}...)
	if err := cmd.Run(); err != nil {
		return err
	}

	// Disable launcher service
	cmd = exec.CommandContext(ctx, "systemctl", []string{"disable", serviceName}...)
	if err := cmd.Run(); err != nil {
		return err
	}

	// Tell the appropriate package manager to remove launcher
	if _, err := os.Stat("/usr/bin/dpkg"); err == nil {
		cmd = exec.CommandContext(ctx, "/usr/bin/dpkg", []string{"--purge", packageName}...)
		if err := cmd.Run(); err != nil {
			return err
		}
	} else if _, err := os.Stat("/bin/rpm"); err == nil {
		cmd = exec.CommandContext(ctx, "/bin/rpm", []string{"-e", packageName}...)
		if err := cmd.Run(); err != nil {
			return err
		}
	}

	dirsToRemove := []string{
		fmt.Sprintf("/var/%s", identifier),
		fmt.Sprintf("/etc/%s", identifier),
	}

	// Now remove the local dirs used for launcher/osquery binaries and app data
	for _, dir := range dirsToRemove {
		if err := os.RemoveAll(dir); err != nil {
			fmt.Printf("error removing dir %s: %s\n", dir, err)
		}
	}

	return nil
}
