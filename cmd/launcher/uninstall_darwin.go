//go:build darwin
// +build darwin

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

func removeLauncher(ctx context.Context, identifier string) error {
	if identifier == "" {
		// Ensure identifier is non-empty and use the default if none provided
		identifier = "kolide-k2"
	}

	launchDaemonPList := fmt.Sprintf("/Library/LaunchDaemons/com.%s.launcher.plist", identifier)
	launchCtlPath := "/bin/launchctl"
	launchCtlArgs := []string{"unload", launchDaemonPList}

	cmd := exec.CommandContext(ctx, launchCtlPath, launchCtlArgs...)
	if err := cmd.Run(); err != nil {
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

	return nil
}
