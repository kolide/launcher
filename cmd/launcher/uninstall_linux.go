//go:build linux
// +build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/kolide/launcher/ee/allowedcmd"
)

func removeLauncher(ctx context.Context, identifier string) error {
	if strings.TrimSpace(identifier) == "" {
		// Ensure identifier is non-empty and use the default if none provided
		identifier = "kolide-k2"
	}

	serviceName := fmt.Sprintf("launcher.%s", identifier)
	packageName := fmt.Sprintf("launcher-%s", identifier)

	// Stop and disable launcher service
	cmd, err := allowedcmd.Systemctl(ctx, []string{"disable", "--now", serviceName}...)
	if err != nil {
		fmt.Printf("could not find systemctl: %s\n", err)
		return err
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		// Don't exit. Log and move on to the next uninstall command
		fmt.Printf("error occurred while stopping/disabling launcher service, systemctl output %s: err: %s\n", string(out), err)
	}

	// Tell the appropriate package manager to remove launcher
	if cmd, err := allowedcmd.Dpkg(ctx, []string{"--purge", packageName}...); err == nil {
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("error occurred while running dpkg --purge, output %s: err: %s\n", string(out), err)
		}
	} else if cmd, err := allowedcmd.Rpm(ctx, []string{"-e", packageName}...); err == nil {
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("error occurred while running rpm -e, output %s: err: %s\n", string(out), err)
		}
	} else {
		return errors.New("unsupported package manager")
	}

	pathsToRemove := []string{
		fmt.Sprintf("/var/%s", identifier),
		fmt.Sprintf("/etc/%s", identifier),
		fmt.Sprintf("/usr/local/%s", identifier),
	}

	// Now remove the paths used for launcher/osquery binaries and app data
	for _, path := range pathsToRemove {
		if err := os.RemoveAll(path); err != nil {
			fmt.Printf("error removing path %s: %s\n", path, err)
		}
	}

	fmt.Println("Kolide launcher uninstalled successfully")

	return nil
}
