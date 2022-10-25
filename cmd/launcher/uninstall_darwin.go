//go:build darwin
// +build darwin

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
)

func removeLauncher(ctx context.Context, logger log.Logger, identifier string) error {
	if identifier == "" {
		// Ensure identifier is non-empty and use the default if none provided
		identifier = "kolide-k2"
	}

	launchDaemonPList := fmt.Sprintf("/Library/LaunchDaemons/com.%s.launcher.plist", identifier)
	launchCtlPath := "/bin/launchctl"
	launchCtlArgs := []string{"unload", launchDaemonPList}

	if _, err := tablehelpers.Exec(ctx, logger, 30, []string{launchCtlPath}, launchCtlArgs); err != nil {
		return err
	}

	// Now we can delete the plist which controls the launcher daemon
	if err := os.Remove(launchDaemonPList); err != nil {
		fmt.Printf("error removing file %s: %s\n", launchDaemonPList, err)
	}

	dirsToRemove := []string{
		fmt.Sprintf("/usr/local/%s", identifier),
		fmt.Sprintf("/etc/%s", identifier),
		fmt.Sprintf("/var/%s", identifier),
	}

	// Now remove the local dirs used for launcher/osquery binaries and app data
	for _, dir := range dirsToRemove {
		if err := os.RemoveAll(dir); err != nil {
			fmt.Printf("error removing dir %s: %s\n", dir, err)
		}
	}

	return nil
}
