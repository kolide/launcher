package flareconsumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/debug/checkups"
	"github.com/kolide/launcher/pkg/debug/shipper"
)

const (
	// Identifier for this consumer.
	FlareSubsystem = "flare"
)

type FlareConsumer struct {
	lastFlareTime time.Time
	flarer        flarer
	knapsack      types.Knapsack
	// newFlareStream is assigned to a field so it can be mocked in tests
	newFlareStream func(note string) (io.WriteCloser, error)
}

type flarer interface {
	RunFlare(ctx context.Context, k types.Knapsack, flareStream io.WriteCloser) error
}

type FlareRunner struct{}

func (f *FlareRunner) RunFlare(ctx context.Context, k types.Knapsack, flareStream io.WriteCloser) error {
	return checkups.RunFlare(ctx, k, flareStream, checkups.InSituEnvironment)
}

func New(knapsack types.Knapsack) *FlareConsumer {
	return &FlareConsumer{
		flarer:   &FlareRunner{},
		knapsack: knapsack,
		newFlareStream: func(note string) (io.WriteCloser, error) {
			return shipper.New(knapsack, shipper.WithNote(note))
		},
	}
}

func (fc *FlareConsumer) Do(data io.Reader) error {
	if time.Since(fc.lastFlareTime) < 5*time.Minute {
		return nil
	}
	fc.lastFlareTime = time.Now()

	if fc.flarer == nil {
		return errors.New("flarer is nil")
	}

	flareData := struct {
		Note string `json:"note"`
	}{}

	if err := json.NewDecoder(data).Decode(&flareData); err != nil {
		return fmt.Errorf("failed to decode key-value json: %w", err)
	}

	flareStream, err := fc.newFlareStream(flareData.Note)
	if err != nil {
		return fmt.Errorf("failed to create flare stream: %w", err)
	}
	return fc.flarer.RunFlare(context.Background(), fc.knapsack, flareStream)
}
