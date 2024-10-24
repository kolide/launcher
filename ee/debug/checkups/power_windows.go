//go:build windows
// +build windows

package checkups

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"

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
	hideWindow(powerCfgCmd)
	if out, err := powerCfgCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("running powercfg.exe: error %w, output %s", err, string(out))
	}

	sprHandle, err := os.Open(tmpFilePath)
	if err != nil {
		return fmt.Errorf("opening system power report: %w", err)
	}
	defer sprHandle.Close()

	extraZip := zip.NewWriter(extraWriter)
	defer extraZip.Close()

	zippedPowerReport, err := extraZip.Create("power.html")
	if err != nil {
		return fmt.Errorf("creating power report zip file: %w", err)
	}

	if _, err := io.Copy(zippedPowerReport, sprHandle); err != nil {
		return fmt.Errorf("copying system power report: %w", err)
	}

	powerCfgSleepStatesCmd, err := allowedcmd.Powercfg(ctx, "/availablesleepstates")
	if err != nil {
		return fmt.Errorf("creating powercfg sleep states command: %w", err)
	}

	hideWindow(powerCfgSleepStatesCmd)
	availableSleepStatesOutput, err := powerCfgSleepStatesCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("running powercfg.exe for sleep states: error %w, output %s", err, string(availableSleepStatesOutput))
	}

	zippedSleepStates, err := extraZip.Create("available_sleep_states.txt")
	if err != nil {
		return fmt.Errorf("creating available sleep states output file: %w", err)
	}

	if _, err := zippedSleepStates.Write(availableSleepStatesOutput); err != nil {
		return fmt.Errorf("writing available sleep states output file: %w", err)
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
