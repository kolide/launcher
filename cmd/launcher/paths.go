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

// getAppBinaryPaths returns the platform specific path where binaries are installed
func getAppBinaryPaths() []string {
	var paths []string
	switch runtime.GOOS {
	case "darwin":
		paths = []string{
			filepath.Join(defaultBinDirectoryPath, "Kolide.app", "Contents", "MacOS", "launcher"),
		}
	case "linux":
		paths = []string{
			filepath.Join(defaultBinDirectoryPath, "launcher"),
		}
	case "windows":
		paths = []string{
			filepath.Join(defaultBinDirectoryPath, "launcher.exe"),
		}
	}
	return paths
}

// getFilepaths returns a list of file paths matching the pattern
func getFilepaths(elem ...string) []string {
	fileGlob := filepath.Join(elem...)
	filepaths, err := filepath.Glob(fileGlob)

	if err == nil && len(filepaths) > 0 {
		return filepaths
	}

	return nil
}
