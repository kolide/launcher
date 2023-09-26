package flareconsumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/debug/checkups"
	"github.com/kolide/launcher/pkg/debug/shipper"
)

const (
	// Identifier for this consumer.
	FlareSubsystem = "flare"
)

type FlareConsumer struct {
	flarer   flarer
	knapsack types.Knapsack
	// newFlareStream is assigned to a field so it can be mocked in tests
	newFlareStream func(note, uploadURL string) io.WriteCloser
}

type flarer interface {
	RunFlare(ctx context.Context, k types.Knapsack, flareStream io.WriteCloser, environment checkups.RuntimeEnvironmentType) error
}

func New(knapsack types.Knapsack) *FlareConsumer {
	return &FlareConsumer{
		flarer:   &FlareRunner{},
		knapsack: knapsack,
		newFlareStream: func(note, uploadURL string) io.WriteCloser {
			return shipper.New(log.NewNopLogger(), knapsack, note, shipper.WithUploadURL(uploadURL))
		},
	}
}

func (fc *FlareConsumer) Do(data io.Reader) error {
	if fc.flarer == nil {
		return errors.New("flarer is nil")
	}

	flareData := struct {
		Note      string `json:"note"`
		UploadURL string `json:"upload_url"`
	}{}

	if err := json.NewDecoder(data).Decode(&flareData); err != nil {
		return fmt.Errorf("failed to decode key-value json: %w", err)
	}

	return fc.flarer.RunFlare(context.Background(), fc.knapsack, fc.newFlareStream(flareData.Note, flareData.UploadURL), checkups.InSituEnvironment)
}
