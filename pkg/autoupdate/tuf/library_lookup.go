package tuf

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/autoupdate"
)

type BinaryUpdateInfo struct {
	Path    string
	Version string
}

// CheckOutLatest returns the path to the latest downloaded executable for our binary, as well
// as its version.
func CheckOutLatest(binary autoupdatableBinary, rootDirectory string, updateDirectory string, channel string, logger log.Logger) (*BinaryUpdateInfo, error) {
	if updateDirectory == "" {
		updateDirectory = defaultLibraryDirectory(rootDirectory)
	}

	update, err := findExecutableFromRelease(binary, LocalTufDirectory(rootDirectory), channel, updateDirectory)
	if err == nil {
		return update, nil
	}

	level.Debug(logger).Log("msg", "could not find executable from release", "err", err)

	// If we can't find the specific release version that we should be on, then just return the executable
	// with the most recent version in the library
	return mostRecentVersion(binary, updateDirectory)
}

// findExecutableFromRelease looks at our local TUF repository to find the release for our
// given channel. If it's already downloaded, then we return its path and version.
func findExecutableFromRelease(binary autoupdatableBinary, tufRepositoryLocation string, channel string, baseUpdateDirectory string) (*BinaryUpdateInfo, error) {
	// Initialize a read-only TUF metadata client to parse the data we already have downloaded about releases.
	metadataClient, err := readOnlyTufMetadataClient(tufRepositoryLocation)
	if err != nil {
		return nil, errors.New("could not initialize TUF client, cannot find release")
	}

	// From already-downloaded metadata, look for the release version
	targets, err := metadataClient.Targets()
	if err != nil {
		return nil, fmt.Errorf("could not get target: %w", err)
	}

	targetName, _, err := findRelease(binary, targets, channel)
	if err != nil {
		return nil, fmt.Errorf("could not find release: %w", err)
	}

	targetPath, targetVersion := pathToTargetVersionExecutable(binary, targetName, baseUpdateDirectory)
	if autoupdate.CheckExecutable(context.TODO(), targetPath, "--version") != nil {
		return nil, fmt.Errorf("version %s from target %s either not yet downloaded or corrupted: %w", targetVersion, targetName, err)
	}

	return &BinaryUpdateInfo{
		Path:    targetPath,
		Version: targetVersion,
	}, nil
}

// mostRecentVersion returns the path to the most recent, valid version available in the library for the
// given binary, along with its version.
func mostRecentVersion(binary autoupdatableBinary, baseUpdateDirectory string) (*BinaryUpdateInfo, error) {
	// Pull all available versions from library
	validVersionsInLibrary, _, err := sortedVersionsInLibrary(binary, baseUpdateDirectory)
	if err != nil {
		return nil, fmt.Errorf("could not get sorted versions in library for %s: %w", binary, err)
	}

	// No valid versions in the library
	if len(validVersionsInLibrary) < 1 {
		return nil, errors.New("no versions in library")
	}

	// Versions are sorted in ascending order -- return the last one
	mostRecentVersionInLibraryRaw := validVersionsInLibrary[len(validVersionsInLibrary)-1]
	versionDir := filepath.Join(updatesDirectory(binary, baseUpdateDirectory), mostRecentVersionInLibraryRaw)
	return &BinaryUpdateInfo{
		Path:    executableLocation(versionDir, binary),
		Version: mostRecentVersionInLibraryRaw,
	}, nil
}
