package keyvalueconsumer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/kolide/launcher/pkg/agent/types"
)

type KeyValueConsumer struct {
	updater types.Updater
}

func New(updater types.Updater) *KeyValueConsumer {
	c := &KeyValueConsumer{
		updater: updater,
	}

	return c
}

func (c *KeyValueConsumer) Update(data io.Reader) error {
	if c == nil {
		return errors.New("key value consumer is nil")
	}

	var kvPairs map[string]string
	if err := json.NewDecoder(data).Decode(&kvPairs); err != nil {
		return fmt.Errorf("failed to decode key-value json: %w", err)
	}

	// Turn the map into a slice of key, value, ... and send it to the thing storing this data
	_, err := c.updater.Update(kvPairs)

	return err
}
