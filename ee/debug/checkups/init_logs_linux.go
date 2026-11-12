package checkups

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/kolide/launcher/v2/ee/agent/types"
	"github.com/kolide/launcher/v2/ee/allowedcmd"
)

func writeInitLogs(ctx context.Context, k types.Knapsack, logZip *zip.Writer) error {
	serviceName := fmt.Sprintf("launcher.%s.service", k.Identifier())
	// journald may not prune logs, so pull last lines to avoid adding too much
	cmd, err := allowedcmd.Journalctl.Cmd(ctx, "-u", serviceName, "-o", "short-iso", "--lines", strconv.Itoa(maxInitLogLines))
	if err != nil {
		return fmt.Errorf("creating journalctl command: %w", err)
	}

	output, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("creating linux_journalctl_launcher_logs.json: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting journalctl: %w", err)
	}

	if err := addStreamToZip(logZip, "linux_journalctl_launcher_logs.json", time.Now(), output); err != nil {
		_ = cmd.Process.Kill()
		return errors.Join(err, cmd.Wait())
	}
	return cmd.Wait()
}
