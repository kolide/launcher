//go:build !windows
// +build !windows

package checkups

import (
	"archive/zip"
	"fmt"

	"github.com/kolide/launcher/pkg/launcher"
)

func gatherInstallerInfo(z *zip.Writer, _ string) error {
	configDir := launcher.DefaultPath(launcher.EtcDirectory)
	installerInfoPath := fmt.Sprintf("%s/installer-info.json", configDir)

	return addFileToZip(z, installerInfoPath)
}
