package checkups

import (
	"archive/zip"
	"context"
	"fmt"

	"github.com/kolide/launcher/ee/allowedcmd"
)

func writeInitLogs(ctx context.Context, logZip *zip.Writer) error {
	cmdStr := `Get-WinEvent -FilterHashtable @{LogName='Application'; ProviderName='launcher'} | ConvertTo-Json`
	cmd, err := allowedcmd.Powershell(ctx, cmdStr)
	if err != nil {
		return fmt.Errorf("creating powershell command: %w", err)
	}

	outFile, err := logZip.Create("windows_launcher_events.json")
	if err != nil {
		return fmt.Errorf("creating windows_launcher_events.json: %w", err)
	}

	cmd.Stderr = outFile
	cmd.Stdout = outFile

	return cmd.Run()
}
