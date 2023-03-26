package flags

import (
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/agent/types"
)

type storedFlagValues struct {
	logger          log.Logger
	agentFlagsStore types.KVStore
}

func NewStoredFlagValues(logger log.Logger, agentFlagsStore types.KVStore) *storedFlagValues {
	s := &storedFlagValues{
		logger:          logger,
		agentFlagsStore: agentFlagsStore,
	}

	return s
}

// Set stores the value for a key.
func (f *storedFlagValues) Set(key FlagKey, value []byte) error {
	return f.agentFlagsStore.Set([]byte(key), value)
}

// Get retrieves the stored value for a key.
func (f *storedFlagValues) Get(key FlagKey) ([]byte, bool) {
	value, err := f.agentFlagsStore.Get([]byte(key))
	if err != nil {
		level.Debug(f.logger).Log("msg", "failed to get stored key", "key", key, "err", err)
		return nil, false
	}

	if value == nil {
		// We didn't find a key-value
		return nil, false
	}

	return value, true
}

// Update replaces data in the key-value store.
func (f *storedFlagValues) Update(pairs ...string) ([]string, error) {
	return f.agentFlagsStore.Update(pairs...)
}
