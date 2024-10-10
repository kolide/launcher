package tuf

import (
	"fmt"
	"os"

	"github.com/theupdateframework/go-tuf/v2/metadata/config"
	"github.com/theupdateframework/go-tuf/v2/metadata/updater"
)

// readOnlyTufMetadataClient returns a metadata client that can read already-downloaded
// metadata but will not download new metadata.
func readOnlyTufMetadataClient(tufRepositoryLocation string) (*updater.Updater, error) {
	// Initialize a read-only TUF metadata client to parse the data we already have about releases
	if _, err := os.Stat(tufRepositoryLocation); os.IsNotExist(err) {
		return nil, fmt.Errorf("local TUF dir doesn't exist, cannot create read-only client: %w", err)
	}

	// Create our TUF client
	cfg, err := config.New("none", rootJson)
	if err != nil {
		return nil, fmt.Errorf("creating TUF config: %w", err)
	}
	cfg.LocalMetadataDir = tufRepositoryLocation
	cfg.DisableLocalCache = true // don't update our TUF repository
	cfg.UnsafeLocalMode = true   // don't perform any downloads
	client, err := updater.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating TUF client: %w", err)
	}

	if err := client.Refresh(); err != nil {
		return nil, fmt.Errorf("loading local TUF metadata: %w", err)
	}

	return client, nil
}
