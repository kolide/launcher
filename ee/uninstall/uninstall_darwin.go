package uninstall

import (
	"context"
	"fmt"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
)

func disableAutoStart(ctx context.Context, k types.Knapsack) error {
	launchDaemonPList := fmt.Sprintf("/Library/LaunchDaemons/com.%s.launcher.plist", k.Identifier())
	launchCtlArgs := []string{"unload", launchDaemonPList}

	launchctlCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd, err := allowedcmd.Launchctl(launchctlCtx, launchCtlArgs...)
	if err != nil {
		return fmt.Errorf("could create launchctl cmd: %w", err)
	}

	if out, err := cmd.Output(); err != nil {
		return fmt.Errorf("error occurred while unloading launcher daemon, launchctl output %s: err: %w", out, err)
	}

	return nil
}
