package commandtracker

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/kolide/launcher/pkg/agent/types"
)

const (
	maxTrackedIds = 10
	idsKey        = "ids"
)

type controlCommandMessage struct {
	Id         string `json:"id"`
	ValidUntil int64  `json:"valid_until"` // timestamp
}

func parseControlCommandMessage(data io.Reader) (*controlCommandMessage, error) {
	var msg controlCommandMessage
	if err := json.NewDecoder(data).Decode(&msg); err != nil {
		return nil, fmt.Errorf("decoding command message json: %w", err)
	}

	if msg.Id == "" {
		return nil, fmt.Errorf("missing id field")
	}

	if msg.ValidUntil == 0 {
		return nil, fmt.Errorf("missing valid_until field")
	}

	return &msg, nil
}

func (c *controlCommandMessage) isExpired() bool {
	return time.Unix(c.ValidUntil, 0).Before(time.Now())
}

type CommandTracker struct {
	store        types.KVStore
	processedIds []string
}

func New(store types.KVStore) (*CommandTracker, error) {
	c := &CommandTracker{
		store: store,
	}

	if store == nil {
		return nil, fmt.Errorf("store is nil")
	}

	idsRaw, err := c.store.Get([]byte(idsKey))
	if err != nil {
		return nil, fmt.Errorf("accessing store: %w", err)
	}

	if idsRaw == nil {
		c.processedIds = make([]string, 0)
		return c, nil
	}

	if err := json.Unmarshal(idsRaw, &c.processedIds); err != nil {
		// if we failed to unmarshall ids, just restart
		c.processedIds = make([]string, 0)
	}

	return c, nil
}

func (c *CommandTracker) ProcessCommand(data io.Reader, processFunc func(io.Reader) error) error {
	msg, err := parseControlCommandMessage(data)
	if err != nil {
		return err
	}

	if msg.isExpired() {
		return nil
	}

	for _, id := range c.processedIds {
		if id == msg.Id {
			return nil
		}
	}

	// got a new id
	if len(c.processedIds) >= maxTrackedIds {
		overage := int(math.Abs(float64(maxTrackedIds - len(c.processedIds))))
		c.processedIds = append(c.processedIds[overage:], msg.Id)
	} else {
		c.processedIds = append(c.processedIds, msg.Id)
	}

	// save processed ids
	idsRaw, err := json.Marshal(c.processedIds)
	if err != nil {
		return fmt.Errorf("marshalling processed ids: %w", err)
	}

	if err := c.store.Set([]byte(idsKey), idsRaw); err != nil {
		return fmt.Errorf("storing processed ids: %w", err)
	}

	// process the command
	return processFunc(data)
}
