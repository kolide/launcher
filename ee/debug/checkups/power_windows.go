//go:build windows
// +build windows

package checkups

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/allowedcmd"
)

type powerCheckup struct{}

func (p *powerCheckup) Name() string {
	return "Power Report"
}

func (p *powerCheckup) Run(ctx context.Context, extraWriter io.Writer) error {
	// Create a temporary file for powercfg to write its output to
	tmpFilePath := agent.TempPath("launcher-checkup-spr.html")
	defer os.Remove(tmpFilePath)

	// See: https://learn.microsoft.com/en-us/windows-hardware/design/device-experiences/powercfg-command-line-options#option_systempowerreport
	powerCfgCmd, err := allowedcmd.Powercfg(ctx, "/systempowerreport", "/output", tmpFilePath)
	if err != nil {
		return fmt.Errorf("creating powercfg command: %w", err)
	}
	hideWindow(powerCfgCmd.Cmd)
	if out, err := powerCfgCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("running powercfg.exe: error %w, output %s", err, string(out))
	}

	extraZip := zip.NewWriter(extraWriter)
	defer extraZip.Close()

	// Add the power report using addFileToZip
	if err := addFileToZip(extraZip, tmpFilePath); err != nil {
		return fmt.Errorf("adding power report to zip: %w", err)
	}

	// Get available sleep states
	powerCfgSleepStatesCmd, err := allowedcmd.Powercfg(ctx, "/availablesleepstates")
	if err != nil {
		return fmt.Errorf("creating powercfg sleep states command: %w", err)
	}

	hideWindow(powerCfgSleepStatesCmd.Cmd)
	availableSleepStatesOutput, err := powerCfgSleepStatesCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("running powercfg.exe for sleep states: error %w, output %s", err, string(availableSleepStatesOutput))
	}

	// Add sleep states using addStreamToZip
	if err := addStreamToZip(extraZip, "available_sleep_states.txt", time.Now(), bytes.NewReader(availableSleepStatesOutput)); err != nil {
		return fmt.Errorf("adding sleep states stream to zip: %w", err)
	}

	// Get power events
	eventFilter := `Get-Winevent -FilterHashtable @{LogName='System'; ProviderName='Microsoft-Windows-Power-Troubleshooter','Microsoft-Windows-Kernel-Power'} -MaxEvents 500 | Select-Object @{name='Time'; expression={$_.TimeCreated.ToString("O")}},Id,LogName,ProviderName,Message,TimeCreated | ConvertTo-Json`
	getWinEventCmd, err := allowedcmd.Powershell(ctx, eventFilter)
	if err != nil {
		return fmt.Errorf("creating powershell get-winevent command: %w", err)
	}

	powerEventsOutput, err := getWinEventCmd.Output()
	if err != nil {
		return fmt.Errorf("running get-winevent command: %w, output %s", err, string(powerEventsOutput))
	}

	// Add power events using addStreamToZip
	if err := addStreamToZip(extraZip, "windows_power_events.json", time.Now(), bytes.NewReader(powerEventsOutput)); err != nil {
		return fmt.Errorf("adding power events to zip: %w", err)
	}

	return nil
}

func (p *powerCheckup) ExtraFileName() string {
	return "power.zip"
}

func (p *powerCheckup) Status() Status {
	return Informational
}

func (p *powerCheckup) Summary() string {
	return ""
}

func (p *powerCheckup) Data() any {
	return nil
}
