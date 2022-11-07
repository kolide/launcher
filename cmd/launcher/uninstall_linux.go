//go:build linux
// +build linux

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func removeLauncher(ctx context.Context, identifier string) error {
	if strings.TrimSpace(identifier) == "" {
		identifier = "kolide-k2"
	}

	serviceName := fmt.Sprintf("launcher.%s", identifier)
	packageName := fmt.Sprintf("launcher-%s", identifier)

	// Stop and disable launcher service
	cmd := exec.CommandContext(ctx, "systemctl", []string{"disable", "--now", serviceName}...)
	if err := cmd.Run(); err != nil {
		return err
	}

	fileExists := func(f string) bool {
		if _, err := os.Stat(f); err == nil {
			return true
		}
		return false
	}

	// Tell the appropriate package manager to remove launcher
	switch {
	case fileExists("/usr/bin/dpkg"):
		cmd = exec.CommandContext(ctx, "/usr/bin/dpkg", []string{"--purge", packageName}...)
		if err := cmd.Run(); err != nil {
			return err
		}
	case fileExists("/bin/rpm"):
		cmd = exec.CommandContext(ctx, "/bin/rpm", []string{"-e", packageName}...)
		if err := cmd.Run(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported package manager")
	}

	pathsToRemove := []string{
		fmt.Sprintf("/var/%s", identifier),
		fmt.Sprintf("/etc/%s", identifier),
	}

	// Now remove the paths used for launcher/osquery binaries and app data
	for _, path := range pathsToRemove {
		if err := os.RemoveAll(path); err != nil {
			fmt.Printf("error removing path %s: %s\n", path, err)
		}
	}

	return nil
}
