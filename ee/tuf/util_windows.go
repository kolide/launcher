//go:build windows
// +build windows

package tuf

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

var likelyRootDirPaths = []string{
	"C:\\ProgramData\\Kolide\\Launcher-kolide-k2\\data",
	"C:\\Program Files\\Kolide\\Launcher-kolide-k2\\data",
}

// executableLocation returns the path to the executable in `updateDirectory`.
func executableLocation(updateDirectory string, binary autoupdatableBinary) string {
	return filepath.Join(updateDirectory, fmt.Sprintf("%s.exe", binary))
}

// checkExecutablePermissions checks whether a specific file looks
// like it's executable. This is used in evaluating whether something
// is an updated version.
//
// Windows does not have executable bits, so we omit those. And
// instead check the file extension.
func checkExecutablePermissions(potentialBinary string) error {
	if potentialBinary == "" {
		return errors.New("empty string isn't executable")
	}
	stat, err := os.Stat(potentialBinary)
	switch {
	case os.IsNotExist(err):
		return errors.New("no such file")
	case err != nil:
		return fmt.Errorf("statting file: %w", err)
	case stat.IsDir():
		return errors.New("is a directory")
	case !strings.HasSuffix(potentialBinary, ".exe"):
		return errors.New("not executable")
	}

	return nil
}

// determineRootDirectory is used specifically for windows deployments to override the
// configured root directory if another one containing a launcher DB already exists
func DetermineRootDirectoryOverride(slogger *slog.Logger, optsRootDirectory, kolideServerURL string) string {
	// don't mess with the path if this installation isn't pointing to a kolide server URL
	if kolideServerURL != "k2device.kolide.com" && kolideServerURL != "k2device-preprod.kolide.com" {
		return optsRootDirectory
	}

	optsDBLocation := filepath.Join(optsRootDirectory, "launcher.db")
	dbExists, err := nonEmptyFileExists(optsDBLocation)
	// If we get an unknown error, back out from making any options changes. This is an
	// unlikely path but doesn't feel right updating the rootDirectory without knowing what's going
	// on here
	if err != nil {
		slogger.Log(context.TODO(), slog.LevelError,
			"encountered error checking for pre-existing launcher.db",
			"location", optsDBLocation,
			"err", err,
		)

		return optsRootDirectory
	}

	// database already exists in configured root directory, keep that
	if dbExists {
		return optsRootDirectory
	}

	// we know this is a fresh install with no launcher.db in the configured root directory,
	// check likely locations and return updated rootDirectory if found
	for _, path := range likelyRootDirPaths {
		if path == optsRootDirectory { // we already know this does not contain an enrolled DB
			continue
		}

		testingLocation := filepath.Join(path, "launcher.db")
		dbExists, err := nonEmptyFileExists(testingLocation)
		if err == nil && dbExists {
			return path
		}

		if err != nil {
			slogger.Log(context.TODO(), slog.LevelWarn,
				"encountered error checking non-configured locations for launcher.db",
				"opts_location", optsDBLocation,
				"tested_location", testingLocation,
				"err", err,
			)

			continue
		}
	}

	// if all else fails, return the originally configured rootDirectory -
	// this is expected for devices that are truly installing from MSI for the first time
	return optsRootDirectory
}

func nonEmptyFileExists(path string) (bool, error) {
	fileInfo, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return fileInfo.Size() > 0, nil
}
