package restartservice

import (
	"context"
	"errors"
	"fmt"

	agentsqlite "github.com/kolide/launcher/ee/agent/storage/sqlite"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/traces"
)

// HealthCheckWriter adheres to the ResultSetter interface
type HealthCheckWriter struct {
	store types.ResultSetter
}

// OpenWriter returns a new health check results writer, creating and initializing
// the database if necessary.
func OpenWriter(ctx context.Context, rootDirectory string) (*HealthCheckWriter, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	store, err := agentsqlite.OpenRW(ctx, rootDirectory, agentsqlite.HealthCheckStore)
	if err != nil {
		return nil, fmt.Errorf("opening healthcheck db in %s: %w", rootDirectory, err)
	}

	s := &HealthCheckWriter{
		store: store,
	}

	return s, nil
}

func (hw *HealthCheckWriter) AddHealthCheckResult(ctx context.Context, timestamp int64, value []byte) error {
	if hw == nil || hw.store == nil {
		return errors.New("store is nil")
	}

	if err := hw.store.AddResult(ctx, timestamp, value); err != nil {
		return fmt.Errorf("adding healthcheck result: %w", err)
	}

	return nil
}

func (r *HealthCheckWriter) Close() error {
	return r.store.Close()
}
