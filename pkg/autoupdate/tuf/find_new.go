package tuf

import (
	"fmt"
	"io"
	"os"

	client "github.com/theupdateframework/go-tuf/client"
	filejsonstore "github.com/theupdateframework/go-tuf/client/filejsonstore"
)

// TODO wrap the client to be a RO client
// Satisfies TUF's RemoteStore interface for our read-only TUF client
type noopRemoteStore struct{}

func newNoopRemoteStore() *noopRemoteStore {
	return &noopRemoteStore{}
}

func (n *noopRemoteStore) GetMeta(name string) (stream io.ReadCloser, size int64, err error) {
	return nil, 0, nil
}

func (n *noopRemoteStore) GetTarget(path string) (stream io.ReadCloser, size int64, err error) {
	return nil, 0, nil
}

// Read-only library manager
type updateLibrary interface {
	MostRecentVersion(binary autoupdatableBinary) (string, error)
	PathToTargetVersionExecutable(binary autoupdatableBinary, targetFilename string) string
	Available(binary autoupdatableBinary, targetFilename string) bool
}

// They check stuff out from the library!
type libraryPatron struct {
	readOnlyLibrary       updateLibrary
	tufRepositoryLocation string
	channel               string
}

func NewUpdateLibraryPatron(rootDirectory string, updateDirectory string, channel string) *libraryPatron {
	if updateDirectory == "" {
		updateDirectory = DefaultLibraryDirectory(rootDirectory)
	}
	readOnlyLibrary := updateLibraryManager{
		baseDir: updateDirectory,
	}
	return &libraryPatron{
		readOnlyLibrary:       &readOnlyLibrary,
		tufRepositoryLocation: LocalTufDirectory(rootDirectory),
		channel:               channel,
	}
}

func (l *libraryPatron) CheckOutLatest(binary autoupdatableBinary) (string, error) {
	releaseVersion, err := l.findExecutableFromRelease(binary)
	if err == nil {
		return releaseVersion, nil
	}

	// If we can't find it, return the executable with the most recent version in the library
	return l.readOnlyLibrary.MostRecentVersion(binary)
}

func (l *libraryPatron) findExecutableFromRelease(binary autoupdatableBinary) (string, error) {
	// Initialize a read-only TUF metadata client to parse the data we already have about releases
	if _, err := os.Stat(l.tufRepositoryLocation); err == os.ErrNotExist {
		return "", fmt.Errorf("local TUF dir doesn't exist yet, cannot find release: %w", err)
	}
	localStore, err := filejsonstore.NewFileJSONStore(l.tufRepositoryLocation)
	if err != nil {
		return "", fmt.Errorf("could not initialize local TUF store: %w", err)
	}
	metadataClient := client.NewClient(localStore, newNoopRemoteStore())

	// From already-downloaded metadata, look for the release version
	targets, err := metadataClient.Targets()
	if err != nil {
		return "", fmt.Errorf("could not get target: %w", err)
	}

	targetName, _, err := findRelease(binary, targets, l.channel)
	if err != nil {
		return "", fmt.Errorf("could not find release: %w", err)
	}

	if !l.readOnlyLibrary.Available(binary, targetName) {
		return "", fmt.Errorf("release target %s for binary %s not yet available in library", targetName, binary)
	}

	return l.readOnlyLibrary.PathToTargetVersionExecutable(binary, targetName), nil
}
