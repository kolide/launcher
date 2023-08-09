//go:build windows
// +build windows

package checkups

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"

	"github.com/kolide/launcher/pkg/agent"
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
	powerCfgCmd := exec.CommandContext(ctx, "powercfg.exe", "/systempowerreport", "/output", tmpFilePath)
	powerCfgCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} // prevents spawning window
	if out, err := powerCfgCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("running powercfg.exe: error %w, output %s", err, string(out))
	}

	sprHandle, err := os.Open(tmpFilePath)
	if err != nil {
		return fmt.Errorf("opening system power report: %w", err)
	}
	defer sprHandle.Close()

	if _, err := io.Copy(extraWriter, sprHandle); err != nil {
		return fmt.Errorf("copying system power report: %w", err)
	}

	return nil
}

func (p *powerCheckup) ExtraFileName() string {
	return "power.html"
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
