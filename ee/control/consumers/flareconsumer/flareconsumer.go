package flareconsumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/go-kit/kit/log"

	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/debug/checkups"
	"github.com/kolide/launcher/pkg/debug/shipper"
)

const (
	// Identifier for this consumer.
	FlareSubsystem   = "flare"
	minFlareInterval = 5 * time.Minute
)

type FlareConsumer struct {
	lastFlareTime time.Time
	flarer        flarer
	knapsack      types.Knapsack
	logger        log.Logger
	// newFlareStream is assigned to a field so it can be mocked in tests
	newFlareStream func(note, uploadRequestURL string) (io.WriteCloser, error)
}

type flarer interface {
	RunFlare(ctx context.Context, k types.Knapsack, flareStream io.WriteCloser) error
}

type FlareRunner struct{}

func (f *FlareRunner) RunFlare(ctx context.Context, k types.Knapsack, flareStream io.WriteCloser) error {
	return checkups.RunFlare(ctx, k, flareStream, checkups.InSituEnvironment)
}

func New(logger log.Logger, knapsack types.Knapsack) *FlareConsumer {
	return &FlareConsumer{
		flarer:   &FlareRunner{},
		knapsack: knapsack,
		newFlareStream: func(note, uploadRequestURL string) (io.WriteCloser, error) {
			return shipper.New(knapsack, shipper.WithNote(note), shipper.WithUploadRequestURL(uploadRequestURL))
		},
	}
}

func (fc *FlareConsumer) Do(data io.Reader) error {
	timeSinceLastFlare := time.Since(fc.lastFlareTime)

	if timeSinceLastFlare < minFlareInterval {
		level.Info(fc.logger).Log(
			"msg", "skipping flare, run too recently",
			"min_flare_interval", fmt.Sprintf("%v minutes", minFlareInterval.Minutes()),
			"time_since_last_flare", fmt.Sprintf("%v minutes", timeSinceLastFlare.Minutes()),
		)
		return nil
	}

	defer func() {
		fc.lastFlareTime = time.Now()
	}()

	if fc.flarer == nil {
		return errors.New("flarer is nil")
	}

	flareData := struct {
		Note             string `json:"note"`
		UploadRequestURL string `json:"upload_request_url"`
	}{}

	if err := json.NewDecoder(data).Decode(&flareData); err != nil {
		return fmt.Errorf("failed to decode key-value json: %w", err)
	}

	flareStream, err := fc.newFlareStream(flareData.Note, flareData.UploadRequestURL)
	if err != nil {
		return fmt.Errorf("failed to create flare stream: %w", err)
	}
	return fc.flarer.RunFlare(context.Background(), fc.knapsack, flareStream)
}
