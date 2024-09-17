package uninstall

import (
	"context"
	"fmt"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/allowedcmd"
)

func disableAutoStart(ctx context.Context, k types.Knapsack) error {
	serviceName := fmt.Sprintf("launcher.%s.service", k.Identifier())
	// the --now flag will disable and stop the service
	cmd, err := allowedcmd.Systemctl(ctx, "disable", "--now", serviceName)
	if err != nil {
		return fmt.Errorf("creating systemctl cmd: %w", err)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("disabling auto start: %w: %s", err, out)
	}

	return nil
}
