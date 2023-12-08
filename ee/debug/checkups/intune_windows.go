//go:build windows
// +build windows

package checkups

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/allowedcmd"
)

type intuneCheckup struct {
	summary string
}

func (i *intuneCheckup) Name() string {
	return "Intune"
}

func (i *intuneCheckup) Run(ctx context.Context, extraWriter io.Writer) error {
	// Other areas of interest: https://learn.microsoft.com/en-us/mem/intune/remote-actions/collect-diagnostics

	zipWriter := zip.NewWriter(extraWriter)
	defer zipWriter.Close()

	if err := agentLogs(zipWriter); err != nil {
		i.summary += fmt.Sprintf("Failed to collect Intune agent logs: %v. ", err)
	}

	if err := installLogs(zipWriter); err != nil {
		i.summary += fmt.Sprintf("Failed to collect Intune install logs: %v. ", err)
	}

	if err := diagnostics(ctx, zipWriter); err != nil {
		i.summary += fmt.Sprintf("Failed to collect Intune diagnostics: %v. ", err)
	}

	i.summary = strings.TrimSpace(i.summary)

	return nil
}

func agentLogs(zipWriter *zip.Writer) error {
	agentLogsPathPattern := filepath.Join(os.Getenv("SYSTEMROOT"), "ProgramData", "Microsoft", "IntuneManagementExtension", "Logs", "*")
	matches, err := filepath.Glob(agentLogsPathPattern)
	if err != nil {
		return fmt.Errorf("globbing for agent logs at %s: %w", agentLogsPathPattern, err)
	}

	for _, match := range matches {
		if err := addFileToZip(zipWriter, match); err != nil {
			return fmt.Errorf("adding %s to zip: %w", match, err)
		}
	}

	return nil
}

func installLogs(zipWriter *zip.Writer) error {
	installLogsPathPattern := filepath.Join(os.Getenv("WINDIR"), "System32", "config", "systemprofile", "AppData", "Local", "mdm", "*.log")
	matches, err := filepath.Glob(installLogsPathPattern)
	if err != nil {
		return fmt.Errorf("globbing for install logs at %s: %w", installLogsPathPattern, err)
	}

	for _, match := range matches {
		if err := addFileToZip(zipWriter, match); err != nil {
			return fmt.Errorf("adding %s to zip: %w", match, err)
		}
	}

	return nil
}

func diagnostics(ctx context.Context, zipWriter *zip.Writer) error {
	tempDir, err := agent.MkdirTemp("mdm-diagnostics")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	tempOutfile := filepath.Join(tempDir, "MdmDiagnosticReport.zip")

	cmd, err := allowedcmd.MdmDiagnosticsTool(ctx, "-zip", tempOutfile)
	if cmd == nil {
		return nil
	} else if err != nil {
		return fmt.Errorf("creating diagnostics command: %w", err)
	}

	if err := addFileToZip(zipWriter, tempOutfile); err != nil {
		return fmt.Errorf("adding diagnostic report to zip: %w", err)
	}

	return nil
}

func (i *intuneCheckup) ExtraFileName() string {
	return "intune.zip"
}

func (i *intuneCheckup) Status() Status {
	return Informational
}

func (i *intuneCheckup) Summary() string {
	return i.summary
}

func (i *intuneCheckup) Data() any {
	return nil
}
