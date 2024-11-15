package checkups

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"runtime"

	"github.com/kolide/launcher/ee/agent/types"
)

type installCheckup struct {
	k types.Knapsack
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

	if err := gatherInstallerInfo(extraZip, i.k.Identifier()); err != nil {
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

	return addFileToZip(z, "/var/log/install.log")
}
