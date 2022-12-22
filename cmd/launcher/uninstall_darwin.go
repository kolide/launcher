//go:build darwin
// +build darwin

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
		// Ensure identifier is non-empty and use the default if none provided
		identifier = "kolide-k2"
	}

	launchDaemonPList := fmt.Sprintf("/Library/LaunchDaemons/com.%s.launcher.plist", identifier)
	launchCtlPath := "/bin/launchctl"
	launchCtlArgs := []string{"unload", launchDaemonPList}

	cmd := exec.CommandContext(ctx, launchCtlPath, launchCtlArgs...)
	if out, err := cmd.Output(); err != nil {
		fmt.Printf("error occurred while unloading launcher daemon, launchctl output %s: err: %s\n", out, err)
		return err
	}

	pathsToRemove := []string{
		launchDaemonPList,
		fmt.Sprintf("/usr/local/%s", identifier),
		fmt.Sprintf("/etc/%s", identifier),
		fmt.Sprintf("/var/%s", identifier),
		fmt.Sprintf("/var/log/%s", identifier),
		fmt.Sprintf("/etc/newsyslog.d/%s.conf", identifier),
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
