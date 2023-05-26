package acceleratecontrolconsumer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/kolide/launcher/ee/control/commandtracker"
	"github.com/kolide/launcher/pkg/agent/types"
)

const (
	// Identifier for this consumer.
	AccelerateControlSubsystem = "accelerate_control"
)

type AccelerateControlConsumer struct {
	overrider      controlRequestIntervalOverrider
	commandTracker *commandtracker.CommandTracker
}

type controlRequestIntervalOverrider interface {
	SetControlRequestIntervalOverride(time.Duration, time.Duration)
}

func New(overrider controlRequestIntervalOverrider, idTrackerStore types.KVStore) (*AccelerateControlConsumer, error) {
	c := &AccelerateControlConsumer{
		overrider: overrider,
	}

	commandTracker, err := commandtracker.New(idTrackerStore)
	if err != nil {
		return nil, fmt.Errorf("creating command tracker: %w", err)
	}

	c.commandTracker = commandTracker
	return c, nil
}

func (c *AccelerateControlConsumer) Update(data io.Reader) error {
	return c.commandTracker.ProcessCommand(data, func(data io.Reader) error {
		if c.overrider == nil {
			return errors.New("control request interval overrider is nil")
		}

		var kvPairs map[string]string
		if err := json.NewDecoder(data).Decode(&kvPairs); err != nil {
			return fmt.Errorf("failed to decode key-value json: %w", err)
		}

		interval, ok := kvPairs["interval"]
		if !ok {
			return errors.New("interval not found in key-value json")
		}

		intervalDuration, err := time.ParseDuration(interval)
		if err != nil {
			return fmt.Errorf("failed to parse interval: %w", err)
		}

		// do the same for duration
		duration, ok := kvPairs["duration"]
		if !ok {
			return errors.New("duration not found in key-value json")
		}

		durationDuration, err := time.ParseDuration(duration)
		if err != nil {
			return fmt.Errorf("failed to parse duration: %w", err)
		}

		c.overrider.SetControlRequestIntervalOverride(intervalDuration, durationDuration)

		return nil
	})
}
