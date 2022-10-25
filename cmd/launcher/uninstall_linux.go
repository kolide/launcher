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

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/pkg/errors"
)

func removeLauncher(ctx context.Context, logger log.Logger, identifier string) error {
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
	if _, err := tablehelpers.Exec(ctx, logger, 30, []string{"systemctl"}, []string{"stop", serviceName}); err != nil {
		return err
	}

	// Disable launcher service
	if _, err := tablehelpers.Exec(ctx, logger, 30, []string{"systemctl"}, []string{"disable", serviceName}); err != nil {
		return err
	}

	// Tell the appropriate package manager to remove launcher
	if isDebianOS {
		if _, err := tablehelpers.Exec(ctx, logger, 30, []string{"dpkg"}, []string{"--purge", packageName}); err != nil {
			return err
		}
	} else {
		if _, err := tablehelpers.Exec(ctx, logger, 30, []string{"rpm"}, []string{"-e", packageName}); err != nil {
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
