package tuf

import (
	"errors"
	"fmt"

	"github.com/go-kit/kit/log"
	"github.com/theupdateframework/go-tuf/client"
)

// Read-only library
type updateLibrary interface {
	IsInstallVersion(binary autoupdatableBinary, targetFilename string) bool
	MostRecentVersion(binary autoupdatableBinary, currentRunningExecutable string) (string, error)
	PathToTargetVersionExecutable(binary autoupdatableBinary, targetFilename string) string
	Available(binary autoupdatableBinary, targetFilename string) bool
}

// libraryLookup performs lookups in the library to find the version of the
// given executable that we want to run.
type libraryLookup struct {
	library               updateLibrary
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
	// We can still find releases in our library, so don't return an error if we can't initialize
	// a client.
	metadataClient, err := readOnlyTufMetadataClient(l.tufRepositoryLocation)
	if err == nil {
		l.metadataClient = metadataClient
	}

	return &l, nil
}

func (l *libraryLookup) CheckOutLatest(binary autoupdatableBinary, currentRunningExecutable string) (string, error) {
	releaseVersion, err := l.findExecutableFromRelease(binary)
	if err == nil {
		return releaseVersion, nil
	}

	// If we can't find the specific release version that we should be on, then just return the executable
	// with the most recent version in the library
	return l.library.MostRecentVersion(binary, currentRunningExecutable)
}

func (l *libraryLookup) findExecutableFromRelease(binary autoupdatableBinary) (string, error) {
	// Initialize a read-only TUF metadata client to parse the data we already have about releases
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
		return "", errors.New("TODO: path to install location")
	}

	if !l.library.Available(binary, targetName) {
		return "", fmt.Errorf("release target %s for binary %s not yet available in library", targetName, binary)
	}

	return l.library.PathToTargetVersionExecutable(binary, targetName), nil
}
