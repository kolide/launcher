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
	cmdStr := `Get-WinEvent -FilterHashtable @{LogName='Application'; ProviderName='launcher'} | ConvertTo-Json`
	cmd, err := allowedcmd.Powershell(ctx, cmdStr)
	if err != nil {
		return fmt.Errorf("creating powershell command: %w", err)
	}

	// Capture command output
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("creating windows_launcher_events.json: %w", err)
	}

	return addStreamToZip(logZip, "windows_launcher_events.json", time.Now(), bytes.NewReader(output))
}
