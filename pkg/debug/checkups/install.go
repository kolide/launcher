package checkups

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
)

type installCheckup struct {
}

func (i *installCheckup) Name() string {
	return "Package Install Logs"
}

func (i *installCheckup) Run(ctx context.Context, extraWriter io.Writer) error {
	extraZip := zip.NewWriter(extraWriter)
	defer extraZip.Close()

	if err := gatherInstallationLogs(extraZip); err != nil {
		return fmt.Errorf("gathering installation logs: %w", err)
	}

	return nil

}

func (i *installCheckup) ExtraFileName() string {
	return "install.zip"
}

func (i *installCheckup) Status() Status {
	return Informational
}

func (i *installCheckup) Summary() string {
	return "N/A"
}

func (i *installCheckup) Data() map[string]any {
	return nil
}

func gatherInstallationLogs(z *zip.Writer) error {
	if runtime.GOOS == "windows" || runtime.GOOS == "linux" {
		return nil
	}

	out, err := z.Create("macos-var-log-install.log")
	if err != nil {
		return fmt.Errorf("creating macos-var-log-install.log in zip: %w", err)
	}

	installLog, err := os.Open("/var/log/install.log")
	if err != nil {
		return fmt.Errorf("opening /var/log/install.log: %w", err)
	}
	defer installLog.Close()

	if _, err := io.Copy(out, installLog); err != nil {
		return fmt.Errorf("writing /var/log/install.log contents to zip: %w", err)
	}

	return nil
}
