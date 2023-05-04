package tuf

import (
	"errors"
	"fmt"

	"github.com/go-kit/kit/log"
	"github.com/theupdateframework/go-tuf/client"
)

// Read-only library
type readOnlyUpdateLibrary interface {
	IsInstallVersion(binary autoupdatableBinary, targetFilename string) bool
	MostRecentVersion(binary autoupdatableBinary) (string, error)
	PathToTargetVersionExecutable(binary autoupdatableBinary, targetFilename string) string
	Available(binary autoupdatableBinary, targetFilename string) bool
}

// libraryLookup performs lookups in the library to find the version of the
// given executable that we want to run.
type libraryLookup struct {
	library               readOnlyUpdateLibrary
	metadataClient        *client.Client
	tufRepositoryLocation string
	channel               string
}

func NewUpdateLibraryLookup(rootDirectory string, updateDirectory string, channel string, logger log.Logger) (*libraryLookup, error) {
	if updateDirectory == "" {
		updateDirectory = DefaultLibraryDirectory(rootDirectory)
	}

	// Set up the library
	r, err := newReadOnlyLibrary(updateDirectory, logger)
	if err != nil {
		return nil, fmt.Errorf("could not create read-only update library: %w", err)
	}

	l := libraryLookup{
		library:               r,
		metadataClient:        nil,
		tufRepositoryLocation: LocalTufDirectory(rootDirectory),
		channel:               channel,
	}

	// Initialize a read-only TUF metadata client to parse the data we already have about releases.
	// We can still pick the most recent version in our library if we can't determine the exact release,
	// so don't return an error if we can't initialize the client.
	if metadataClient, err := readOnlyTufMetadataClient(l.tufRepositoryLocation); err == nil {
		l.metadataClient = metadataClient
	}

	return &l, nil
}

// CheckOutLatest returns the path to the latest downloaded executable for our binary. If the
// installation is the most recent version, then we return an empty string.
func (l *libraryLookup) CheckOutLatest(binary autoupdatableBinary) (string, error) {
	releaseVersion, err := l.findExecutableFromRelease(binary)
	if err == nil {
		return releaseVersion, nil
	}

	// If we can't find the specific release version that we should be on, then just return the executable
	// with the most recent version in the library
	return l.library.MostRecentVersion(binary)
}

// findExecutableFromRelease looks at our local TUF repository to find the release for our
// given channel. If it's already downloaded, then we return its path.
func (l *libraryLookup) findExecutableFromRelease(binary autoupdatableBinary) (string, error) {
	// If we couldn't initialize the metadata client on library lookup creation, then we
	// can't parse our TUF repository now.
	if l.metadataClient == nil {
		return "", errors.New("no TUF client initialized, cannot find release")
	}

	// From already-downloaded metadata, look for the release version
	targets, err := l.metadataClient.Targets()
	if err != nil {
		return "", fmt.Errorf("could not get target: %w", err)
	}

	targetName, _, err := findRelease(binary, targets, l.channel)
	if err != nil {
		return "", fmt.Errorf("could not find release: %w", err)
	}

	if l.library.IsInstallVersion(binary, targetName) {
		return "", nil
	}

	if !l.library.Available(binary, targetName) {
		return "", fmt.Errorf("release target %s for binary %s not yet available in library", targetName, binary)
	}

	return l.library.PathToTargetVersionExecutable(binary, targetName), nil
}
