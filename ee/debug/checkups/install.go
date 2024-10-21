package checkups

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/kolide/launcher/pkg/launcher"
)

type installCheckup struct {
}

func (i *installCheckup) Name() string {
	return "Package Install"
}

func (i *installCheckup) Run(ctx context.Context, extraWriter io.Writer) error {
	extraZip := zip.NewWriter(extraWriter)
	defer extraZip.Close()

	if err := gatherInstallationLogs(extraZip); err != nil {
		return fmt.Errorf("gathering installation logs: %w", err)
	}

	if err := gatherInstallerInfo(extraZip); err != nil {
		return fmt.Errorf("gathering installer info: %w", err)
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

func (i *installCheckup) Data() any {
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

func gatherInstallerInfo(z *zip.Writer) error {
	if runtime.GOOS == "windows" {
		// Installer info is not available on Windows
		return nil
	}

	configDir := launcher.DefaultPath(launcher.EtcDirectory)
	installerInfoPath := fmt.Sprintf("%s/installer-info.json", configDir)

	installerInfoFile, err := os.Open(installerInfoPath)
	if err != nil {
		// If the file doesn't exist, you might want to skip without error
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("opening %s: %w", installerInfoPath, err)
	}
	defer installerInfoFile.Close()

	installerInfoZipPath := "installer-info.json"
	out, err := z.Create(installerInfoZipPath)
	if err != nil {
		return fmt.Errorf("creating %s in zip: %w", installerInfoZipPath, err)
	}

	if _, err := io.Copy(out, installerInfoFile); err != nil {
		return fmt.Errorf("writing %s contents to zip: %w", installerInfoZipPath, err)
	}

	return nil
}
