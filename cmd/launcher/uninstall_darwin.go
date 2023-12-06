//go:build darwin
// +build darwin

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kolide/launcher/ee/allowedcmd"
)

func removeLauncher(ctx context.Context, identifier string) error {
	if strings.TrimSpace(identifier) == "" {
		// Ensure identifier is non-empty and use the default if none provided
		identifier = "kolide-k2"
	}

	launchDaemonPList := fmt.Sprintf("/Library/LaunchDaemons/com.%s.launcher.plist", identifier)
	launchCtlArgs := []string{"unload", launchDaemonPList}

	launchctlCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd, err := allowedcmd.Launchctl(launchctlCtx, launchCtlArgs...)
	if err != nil {
		fmt.Printf("could not find launchctl: %s\n", err)
		return err
	}
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
	pkgUtilcmd, err := allowedcmd.Pkgutil(pkgutiltCtx, "--forget", fmt.Sprintf("com.%s.launcher", identifier))
	if err != nil {
		fmt.Printf("could not find pkgutil: %s\n", err)
		return err
	}

	if out, err := pkgUtilcmd.Output(); err != nil {
		fmt.Printf("error occurred while forgetting package: output %s: err: %s\n", out, err)
		return nil
	}

	fmt.Println("Kolide launcher uninstalled successfully")

	return nil
}
