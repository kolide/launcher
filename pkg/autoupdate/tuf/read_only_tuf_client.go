package tuf

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	client "github.com/theupdateframework/go-tuf/client"
	filejsonstore "github.com/theupdateframework/go-tuf/client/filejsonstore"
)

// readOnlyTufMetadataClient returns a metadata client that can read already-downloaded
// metadata but will not download new metadata.
func readOnlyTufMetadataClient(tufRepositoryLocation string) (*client.Client, error) {
	// Initialize a read-only TUF metadata client to parse the data we already have about releases
	if _, err := os.Stat(tufRepositoryLocation); err == os.ErrNotExist {
		return nil, fmt.Errorf("local TUF dir doesn't exist, cannot create read-only client: %w", err)
	}

	localStore, err := newReadOnlyLocalStore(tufRepositoryLocation)
	if err != nil {
		return nil, fmt.Errorf("could not initialize read-only local TUF store: %w", err)
	}

	metadataClient := client.NewClient(localStore, newNoopRemoteStore())
	if err := metadataClient.Init(rootJson); err != nil {
		return nil, fmt.Errorf("failed to initialize read-only TUF client with root JSON: %w", err)
	}

	return metadataClient, nil
}

// Wraps TUF's FileJSONStore to be read-only
type readOnlyLocalStore struct {
	localFilestore *filejsonstore.FileJSONStore
}

func newReadOnlyLocalStore(tufRepositoryLocation string) (*readOnlyLocalStore, error) {
	localStore, err := filejsonstore.NewFileJSONStore(tufRepositoryLocation)
	if err != nil {
		return nil, fmt.Errorf("could not initialize local TUF store: %w", err)
	}
	return &readOnlyLocalStore{
		localFilestore: localStore,
	}, nil
}

func (r *readOnlyLocalStore) Close() error {
	return r.localFilestore.Close()
}

func (r *readOnlyLocalStore) GetMeta() (map[string]json.RawMessage, error) {
	return r.localFilestore.GetMeta()
}

func (r *readOnlyLocalStore) SetMeta(name string, meta json.RawMessage) error {
	return nil
}

func (r *readOnlyLocalStore) DeleteMeta(name string) error {
	return nil
}

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
