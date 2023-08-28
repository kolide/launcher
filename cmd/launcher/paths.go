package main

import (
	"path/filepath"
	"runtime"

	"github.com/kolide/launcher/pkg/launcher"
)

// setDefaultPaths populates the default file/dir paths
// call this before calling parseOptions if you want to assume these paths exist
func setDefaultPaths() {
	switch runtime.GOOS {
	case "darwin":
		launcher.DefaultRootDirectoryPath = "/var/kolide-k2/k2device.kolide.com/"
		launcher.DefaultEtcDirectoryPath = "/etc/kolide-k2/"
		launcher.DefaultBinDirectoryPath = "/usr/local/kolide-k2/"
		launcher.DefaultConfigFilePath = filepath.Join(launcher.DefaultEtcDirectoryPath, "launcher.flags")
	case "linux":
		launcher.DefaultRootDirectoryPath = "/var/kolide-k2/k2device.kolide.com/"
		launcher.DefaultEtcDirectoryPath = "/etc/kolide-k2/"
		launcher.DefaultBinDirectoryPath = "/usr/local/kolide-k2/"
		launcher.DefaultConfigFilePath = filepath.Join(launcher.DefaultEtcDirectoryPath, "launcher.flags")
	case "windows":
		launcher.DefaultRootDirectoryPath = "C:\\Program Files\\Kolide\\Launcher-kolide-k2\\data"
		launcher.DefaultEtcDirectoryPath = ""
		launcher.DefaultBinDirectoryPath = "C:\\Program Files\\Kolide\\Launcher-kolide-k2\\bin"
		launcher.DefaultConfigFilePath = filepath.Join("C:\\Program Files\\Kolide\\Launcher-kolide-k2\\conf", "launcher.flags")
	}
}
