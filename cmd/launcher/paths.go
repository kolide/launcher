package main

import (
	"path/filepath"
	"runtime"
)

// setDefaultPaths populates the default file/dir paths
// call this before calling parseOptions if you want to assume these paths exist
func setDefaultPaths() {
	switch runtime.GOOS {
	case "darwin":
		defaultRootDirectoryPath = "/var/kolide-k2/k2device.kolide.com/"
		defaultEtcDirectoryPath = "/etc/kolide-k2/"
		defaultBinDirectoryPath = "/usr/local/kolide-k2/"
		defaultConfigFilePath = filepath.Join(defaultEtcDirectoryPath, "launcher.flags")
	case "linux":
		defaultRootDirectoryPath = "/var/kolide-k2/k2device.kolide.com/"
		defaultEtcDirectoryPath = "/etc/kolide-k2/"
		defaultBinDirectoryPath = "/usr/local/kolide-k2/"
		defaultConfigFilePath = filepath.Join(defaultEtcDirectoryPath, "launcher.flags")
	case "windows":
		defaultRootDirectoryPath = "C:\\Program Files\\Kolide\\Launcher-kolide-k2\\data"
		defaultEtcDirectoryPath = ""
		defaultBinDirectoryPath = "C:\\Program Files\\Kolide\\Launcher-kolide-k2\\bin"
		defaultConfigFilePath = filepath.Join("C:\\Program Files\\Kolide\\Launcher-kolide-k2\\conf", "launcher.flags")
	}
}

// windowsAddExe appends ".exe" to the input string when running on Windows
func windowsAddExe(in string) string {
	if runtime.GOOS == "windows" {
		return in + ".exe"
	}

	return in
}
