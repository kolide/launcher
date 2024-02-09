package checkups

import (
	"archive/zip"
	"context"
	"fmt"

	"github.com/kolide/launcher/ee/allowedcmd"
)

func writeInitLogs(ctx context.Context, logZip *zip.Writer) error {
	cmd, err := allowedcmd.Journalctl(ctx, "-u", "launcher.kolide-k2.service", "-o", "json")
	if err != nil {
		return fmt.Errorf("creating journalctl command: %w", err)
	}

	outFile, err := logZip.Create("linux_journalctl_launcher_logs.json")
	if err != nil {
		return fmt.Errorf("creating linux_journalctl_launcher_logs.json: %w", err)
	}

	cmd.Stderr = outFile
	cmd.Stdout = outFile

	return cmd.Run()
}
