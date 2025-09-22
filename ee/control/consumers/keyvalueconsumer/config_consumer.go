package keyvalueconsumer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/kolide/launcher/ee/agent/types"
)

type ConfigConsumer struct {
	updater types.Updater
}

func NewConfigConsumer(updater types.Updater) *ConfigConsumer {
	c := &ConfigConsumer{
		updater: updater,
	}

	return c
}

func (c *ConfigConsumer) Update(data io.Reader) error {
	if c == nil {
		return errors.New("key value consumer is nil")
	}

	var kvPairs map[string]any
	if err := json.NewDecoder(data).Decode(&kvPairs); err != nil {
		return fmt.Errorf("failed to decode key-value json: %w", err)
	}

	kvStringPairs := make(map[string]string)
	for k, v := range kvPairs {
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("unable to marshal value for `%s`: %w", k, err)
		}
		kvStringPairs[k] = string(b)
	}

	// Turn the map into a slice of key, value, ... and send it to the thing storing this data
	_, err := c.updater.Update(kvStringPairs)

	return err
}
