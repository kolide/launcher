package tokenconsumer

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/kolide/launcher/pkg/agent/types"
)

const IngestSubsystem = "ingest"

var ObservabilityIngestTokenKey = []byte("observability_ingest_server_token")

type IngestTokenConsumer struct {
	store types.KVStore
}

type traceSubsystemConfig struct {
	IngestToken string `json:"ingest_token"`
}

func NewIngestTokenConsumer(store types.KVStore) *IngestTokenConsumer {
	return &IngestTokenConsumer{
		store: store,
	}
}

// Update satisfies control.consumer interface
func (i *IngestTokenConsumer) Update(data io.Reader) error {
	var updatedCfg traceSubsystemConfig
	if err := json.NewDecoder(data).Decode(&updatedCfg); err != nil {
		return fmt.Errorf("failed to decode trace subsystem data: %w", err)
	}

	if err := i.store.Set(ObservabilityIngestTokenKey, []byte(updatedCfg.IngestToken)); err != nil {
		return fmt.Errorf("could not store token after update: %w", err)
	}

	return nil
}
