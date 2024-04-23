package restartservice

import (
	"context"
	"fmt"

	agentsqlite "github.com/kolide/launcher/ee/agent/storage/sqlite"
	"github.com/kolide/launcher/ee/agent/types"
)

type (
	healthCheckReader struct {
		store types.ResultFetcher
	}
)

func OpenReader(ctx context.Context, rootDirectory string) (*healthCheckReader, error) {
	store, err := agentsqlite.OpenRO(ctx, rootDirectory, agentsqlite.HealthCheckStore)
	if err != nil {
		return nil, fmt.Errorf("opening healthcheck db in %s: %w", rootDirectory, err)
	}

	return &healthCheckReader{store: store}, nil
}

func (r *healthCheckReader) Close() error {
	return r.store.Close()
}
