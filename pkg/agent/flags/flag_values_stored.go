package flags

import (
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/agent/types"
)

type storedFlagValues struct {
	logger          log.Logger
	agentFlagsStore types.GetterSetter
	keysMap         map[FlagKey][]byte
}

func NewStoredFlagValues(logger log.Logger, agentFlagsStore types.GetterSetter) *storedFlagValues {
	s := &storedFlagValues{
		logger:          logger,
		agentFlagsStore: agentFlagsStore,
		keysMap:         generateKeysMap(),
	}

	return s
}

// generateKeysMap returns a map which associates FlagKeys to the named keys used within a key-value store
func generateKeysMap() map[FlagKey][]byte {
	keysMap := map[FlagKey][]byte{
		DesktopEnabled: []byte("desktop_enabled_v1"),
	}
	return keysMap
}

// lookupKey finds the named key associated with the FlagKey
// If there is none, the string value of key is returned
func (f *storedFlagValues) lookupKey(key FlagKey) []byte {
	lookupKey := []byte(key)
	mappedKey, exists := f.keysMap[key]
	if exists {
		lookupKey = mappedKey
	}
	return lookupKey
}

// Set stores the value for a key.
func (f *storedFlagValues) Set(key FlagKey, value []byte) error {
	lookupKey := f.lookupKey(key)
	return f.agentFlagsStore.Set(lookupKey, value)
}

// Get retrieves the stored value for a key.
func (f *storedFlagValues) Get(key FlagKey) ([]byte, bool) {
	lookupKey := f.lookupKey(key)
	value, err := f.agentFlagsStore.Get(lookupKey)
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
