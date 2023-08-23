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

func (c *AccelerateControlConsumer) Do(data io.Reader) error {
	if c.overrider == nil {
		return errors.New("control request interval overrider is nil")
	}

	accelerateData := struct {
		// expected to come in from contorl server in seconds
		Interval int `json:"interval"`
		Duration int `json:"duration"`
	}{}

	if err := json.NewDecoder(data).Decode(&accelerateData); err != nil {
		return fmt.Errorf("failed to decode key-value json: %w", err)
	}

	c.overrider.SetControlRequestIntervalOverride(
		time.Duration(accelerateData.Interval)*time.Second,
		time.Duration(accelerateData.Duration)*time.Second,
	)

	return nil
}
