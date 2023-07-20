package acceleratecontrolconsumer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

const (
	// Identifier for this consumer.
	AccelerateControlSubsystem = "accelerate_control"
)

type AccelerateControlConsumer struct {
	overrider controlRequestIntervalOverrider
}

type controlRequestIntervalOverrider interface {
	SetControlRequestIntervalOverride(time.Duration, time.Duration)
}

func New(overrider controlRequestIntervalOverrider) *AccelerateControlConsumer {
	return &AccelerateControlConsumer{
		overrider: overrider,
	}
}

func (c *AccelerateControlConsumer) Update(data io.Reader) error {
	if c.overrider == nil {
		return errors.New("control request interval overrider is nil")
	}

	accelerate_data := struct {
		Interval string `json:"interval"`
		Duration string `json:"duration"`
	}{}

	if err := json.NewDecoder(data).Decode(&accelerate_data); err != nil {
		return fmt.Errorf("failed to decode key-value json: %w", err)
	}

	intervalDuration, err := time.ParseDuration(accelerate_data.Interval)
	if err != nil {
		return fmt.Errorf("failed to parse interval: %w", err)
	}

	durationDuration, err := time.ParseDuration(accelerate_data.Duration)
	if err != nil {
		return fmt.Errorf("failed to parse duration: %w", err)
	}

	c.overrider.SetControlRequestIntervalOverride(intervalDuration, durationDuration)

	return nil
}
