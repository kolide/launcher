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

	if found, err := agentLogs(zipWriter); err != nil {
		i.summary += fmt.Sprintf("Failed to collect Intune agent logs: %v. ", err)
	} else if !found {
		i.summary += "Intune agent logs not found. "
	} else {
		i.summary += "Intune agent logs found. "
	}

	if found, err := installLogs(zipWriter); err != nil {
		i.summary += fmt.Sprintf("Failed to collect Intune install logs: %v. ", err)
	} else if !found {
		i.summary += "Intune install logs not found. "
	} else {
		i.summary += "Intune install logs found. "
	}

	i.summary = strings.TrimSpace(i.summary)

	return nil
}

func agentLogs(zipWriter *zip.Writer) (bool, error) {
	agentLogsPathPattern := filepath.Join(os.Getenv("SYSTEMROOT"), "ProgramData", "Microsoft", "IntuneManagementExtension", "Logs", "*")
	matches, err := filepath.Glob(agentLogsPathPattern)
	if err != nil {
		return false, fmt.Errorf("globbing for agent logs at %s: %w", agentLogsPathPattern, err)
	}
	if len(matches) == 0 {
		// Probably a non-Intune machine!
		return false, nil
	}

	for _, match := range matches {
		if err := addFileToZip(zipWriter, match); err != nil {
			return true, fmt.Errorf("adding %s to zip: %w", match, err)
		}
	}

	return true, nil
}

func installLogs(zipWriter *zip.Writer) (bool, error) {
	installLogsPathPattern := filepath.Join(os.Getenv("WINDIR"), "System32", "config", "systemprofile", "AppData", "Local", "mdm", "*.log")
	matches, err := filepath.Glob(installLogsPathPattern)
	if err != nil {
		return false, fmt.Errorf("globbing for install logs at %s: %w", installLogsPathPattern, err)
	}
	if len(matches) == 0 {
		// Probably a non-Intune machine!
		return false, nil
	}

	for _, match := range matches {
		if err := addFileToZip(zipWriter, match); err != nil {
			return true, fmt.Errorf("adding %s to zip: %w", match, err)
		}
	}

	return true, nil
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
