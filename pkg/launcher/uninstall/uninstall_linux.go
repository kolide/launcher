package uninstall

import (
	"context"
	"fmt"

	"github.com/kolide/launcher/ee/allowedcmd"
)

func disableAutoStart(ctx context.Context) error {
	// the --now flag will disable and stop the service
	cmd, err := allowedcmd.Systemctl(ctx, "disable", "--now", "launcher.kolide-k2.service")
	if err != nil {
		return fmt.Errorf("creating systemctl cmd: %w", err)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("disabling auto start: %w: %s", err, out)
	}

	return nil
}
