package main

import (
	"path/filepath"
	"runtime"
)

func getDefaults(defaultRootDir, defaultEtcDir, binDir, defaultConfigFile *string) {
	switch runtime.GOOS {
	case "darwin":
		*defaultRootDir = "/var/kolide-k2/k2device.kolide.com/"
		*defaultEtcDir = "/etc/kolide-k2/"
		*binDir = "/usr/local/kolide-k2/"
		*defaultConfigFile = filepath.Join(*defaultEtcDir, "launcher.flags")
	case "linux":
		*defaultRootDir = "/var/kolide-k2/k2device.kolide.com/"
		*defaultEtcDir = "/etc/kolide-k2/"
		*binDir = "/usr/local/kolide-k2/"
		*defaultConfigFile = filepath.Join(*defaultEtcDir, "launcher.flags")
	case "windows":
		*defaultRootDir = "C:\\Program Files\\Kolide\\Launcher-kolide-k2\\data"
		*defaultEtcDir = ""
		*binDir = "C:\\Program Files\\Kolide\\Launcher-kolide-k2\\bin"
		*defaultConfigFile = filepath.Join("C:\\Program Files\\Kolide\\Launcher-kolide-k2\\conf", "launcher.flags")
	}
}

func getAppBinaryPaths() []string {
	var paths []string
	switch runtime.GOOS {
	case "darwin":
		paths = []string{
			filepath.Join(binDir, "Kolide.app", "Contents", "MacOS", "launcher"),
		}
	case "linux":
		paths = []string{
			filepath.Join(binDir, "launcher.exe"),
		}
	case "windows":
		paths = []string{
			filepath.Join(binDir, "launcher.exe"),
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
