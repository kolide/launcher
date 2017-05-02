package main

import (
	"errors"
	"os"
	"runtime"
)

// OsqueryPlatform is the specific type assigned to osquery platform strings
type OsqueryPlatform string

const (
	Unknown OsqueryPlatform = "unknown"
	Windows OsqueryPlatform = "windows"
	Darwin  OsqueryPlatform = "darwin"
	Ubuntu  OsqueryPlatform = "ubuntu"
	CentOS  OsqueryPlatform = "centos"
)

// DetectPlatform returns the runtime platform, or an error if it cannot
// sufficiently detect.
func DetectPlatform() (OsqueryPlatform, error) {
	switch runtime.GOOS {
	case "windows":
		return Windows, nil
	case "darwin":
		return Darwin, nil
	case "linux":
		return detectLinux()
	default:
		return Unknown, errors.New("unrecognized runtime.GOOS: " + runtime.GOOS)
	}
}

// fileExists checks whether a file exists at the given path
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// detectLinux differentiates between the supported flavors of Linux
func detectLinux() (OsqueryPlatform, error) {
	switch {
	case fileExists("/etc/debian_version"):
		return Ubuntu, nil
	case fileExists("/etc/redhat-release") || fileExists("/etc/centos-relese"):
		return CentOS, nil
	default:
		return Unknown, errors.New("cannot differentiate Linux flavor")
	}
}
