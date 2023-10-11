//go:build darwin
// +build darwin

package uninstall

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

func removeStartScripts() error {
	launchDaemonPList := "/Library/LaunchDaemons/com.kolide-k2.launcher.plist"
	launchCtlPath := "/bin/launchctl"
	launchCtlArgs := []string{"unload", launchDaemonPList}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, launchCtlPath, launchCtlArgs...)
	if out, err := cmd.Output(); err != nil {
		return fmt.Errorf("unloading launcher daemon: %w: %s", err, out)
	}

	return nil
}

func removeInstallation() error {
	cmd := exec.Command("/usr/sbin/pkgutil", "--forget", "com.kolide.k2.launcher")
	if out, err := cmd.Output(); err != nil {
		return fmt.Errorf("pkgutil forgetting package: %w: %s", err, out)
	}
	return nil
}
