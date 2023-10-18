//go:build darwin
// +build darwin

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func removeLauncher(ctx context.Context, identifier string) error {
	if strings.TrimSpace(identifier) == "" {
		// Ensure identifier is non-empty and use the default if none provided
		identifier = "kolide-k2"
	}

	launchDaemonPList := fmt.Sprintf("/Library/LaunchDaemons/com.%s.launcher.plist", identifier)
	launchCtlPath := "/bin/launchctl"
	launchCtlArgs := []string{"unload", launchDaemonPList}

	launchctlCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(launchctlCtx, launchCtlPath, launchCtlArgs...)
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

	removeErr := false

	// Now remove the paths used for launcher/osquery binaries and app data
	for _, path := range pathsToRemove {
		if err := os.RemoveAll(path); err != nil {
			removeErr = true
			fmt.Printf("error removing path %s: %s\n", path, err)
		}
	}

        if removeErr {
                return nil
        }
        
        pkgutiltCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()      
        cmd := exec.CommandContext(pkgutiltCtx, "/usr/sbin/pkgutil", "--forget", fmt.Sprintf("com.%s.launcher", identifier))
        
        if out, err := cmd.Output(); err != nil {
                fmt.Printf("error occurred while forgetting package: output %s: err: %s\n", out, err)
                return nil
	}

	fmt.Println("Kolide launcher uninstalled successfully")

	return nil
}
