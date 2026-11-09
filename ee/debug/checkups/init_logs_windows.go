package checkups

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kolide/launcher/v2/ee/allowedcmd"
)

func writeInitLogs(ctx context.Context, logZip *zip.Writer) error {
	cmdStr := fmt.Sprintf(`Get-WinEvent -FilterHashtable @{LogName='Application'; ProviderName='launcher'} -MaxEvents %d | ConvertTo-Json`, maxInitLogLines)
	cmd, err := allowedcmd.Powershell.Cmd(ctx, cmdStr)
	if err != nil {
		return fmt.Errorf("creating powershell command: %w", err)
	}

	output, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("creating windows_launcher_events.json: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting powershell: %w", err)
	}

	if err := addStreamToZip(logZip, "windows_launcher_events.json", time.Now(), output); err != nil {
		_ = cmd.Process.Kill()
		return errors.Join(err, cmd.Wait())
	}
	return cmd.Wait()
}
