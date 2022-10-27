//go:build linux
// +build linux

package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
)

func removeLauncher(ctx context.Context, identifier string) error {
	if identifier == "" {
		identifier = "kolide-k2"
	}

	isDebianOS, err := isDebian(ctx, logger)
	if err != nil {
		return err
	}

	serviceName := fmt.Sprintf("launcher.%s", identifier)
	packageName := fmt.Sprintf("launcher-%s", identifier)

	// Stop launcher service
	cmd := exec.CommandContext(ctx, "systemctl", []string{"stop", serviceName}...)
	if err := cmd.Run(); err != nil {
		return err
	}

	// Disable launcher service
	cmd := exec.CommandContext(ctx, "systemctl", []string{"disable", serviceName}...)
	if err := cmd.Run(); err != nil {
		return err
	}

	// Tell the appropriate package manager to remove launcher
	if isDebianOS {
		cmd := exec.CommandContext(ctx, "dpkg", []string{"--purge", packageName}...)
		if err := cmd.Run(); err != nil {
			return err
		}
	} else {
		cmd := exec.CommandContext(ctx, "rpm", []string{"-e", packageName}...)
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

func isDebian(ctx context.Context, logger log.Logger) (bool, error) {
	output, err := tablehelpers.Exec(ctx, logger, 30, []string{"cat"}, []string{"/etc/os-release"})
	if err != nil {
		return false, err
	}

	prefix := "ID_LIKE="
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, prefix) {
			return strings.Contains(line, "debian"), nil
		}
	}

	return false, errors.New("unable to determine type of Linux distribution")
}
