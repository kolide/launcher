package checkups

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/kolide/launcher/ee/allowedcmd"
)

func writeInitLogs(ctx context.Context, logZip *zip.Writer) error {
	cmd, err := allowedcmd.Journalctl(ctx, "-u", "launcher.kolide-k2.service")
	if err != nil {
		return fmt.Errorf("creating journalctl command: %w", err)
	}

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("creating linux_journalctl_launcher_logs.json: %w", err)
	}

	return addStreamToZip(logZip, "linux_journalctl_launcher_logs.json", time.Now(), bytes.NewReader(output))
}
