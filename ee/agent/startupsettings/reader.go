package startupsettings

import (
	"context"
	"fmt"
	"log/slog"

	agentsqlite "github.com/kolide/launcher/ee/agent/storage/sqlite"
	"github.com/kolide/launcher/ee/agent/types"
)

type startupSettingsReader struct {
	kvStore types.GetterCloser
}

func OpenReader(ctx context.Context, slogger *slog.Logger, rootDirectory string) (*startupSettingsReader, error) {
	store, err := agentsqlite.OpenRO(ctx, slogger, rootDirectory, agentsqlite.StartupSettingsStore)
	if err != nil {
		return nil, fmt.Errorf("opening startup db in %s: %w", rootDirectory, err)
	}

	return &startupSettingsReader{kvStore: store}, nil
}

// Get retrieves the value for the given flagKey from the startup database
// located in the given rootDirectory.
func (r *startupSettingsReader) Get(flagKey string) (string, error) {
	flagValue, err := r.kvStore.Get([]byte(flagKey))
	if err != nil {
		return "", fmt.Errorf("getting flag value %s: %w", flagKey, err)
	}

	return string(flagValue), nil
}

func (r *startupSettingsReader) Close() error {
	return r.kvStore.Close()
}
