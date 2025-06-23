package flareconsumer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/debug/checkups"
	"github.com/kolide/launcher/ee/debug/shipper"
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
	// newFlareStream is assigned to a field so it can be mocked in tests
	newFlareStream func(note, uploadRequestURL string) (io.WriteCloser, error)
	slogger        *slog.Logger
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
		newFlareStream: func(note, uploadRequestURL string) (io.WriteCloser, error) {
			return shipper.New(knapsack, shipper.WithNote(note), shipper.WithUploadRequestURL(uploadRequestURL))
		},
		slogger: knapsack.Slogger().With("component", FlareSubsystem),
	}
}

func (fc *FlareConsumer) Do(data io.Reader) error {
	// slog needs a ctx
	ctx := context.TODO()

	timeSinceLastFlare := time.Since(fc.lastFlareTime)

	if timeSinceLastFlare < minFlareInterval {
		fc.slogger.Log(ctx, slog.LevelInfo, "skipping flare, run too recently, not retrying",
			"min_flare_interval", fmt.Sprintf("%v minutes", minFlareInterval.Minutes()),
			"time_since_last_flare", fmt.Sprintf("%v minutes", timeSinceLastFlare.Minutes()),
		)
		return nil
	}

	defer func() {
		fc.lastFlareTime = time.Now()
	}()

	if fc.flarer == nil {
		fc.slogger.Log(ctx, slog.LevelError,
			"flarer is nil, not retrying",
		)
		return nil
	}

	flareData := struct {
		Note             string `json:"note"`
		UploadRequestURL string `json:"upload_request_url"`
	}{}

	if err := json.NewDecoder(data).Decode(&flareData); err != nil {
		fc.slogger.Log(ctx, slog.LevelError,
			"failed to decode key-value json, not retrying",
			"err", err,
		)
		return nil
	}

	fc.slogger.Log(ctx, slog.LevelInfo, "received remote flare request",
		"note", flareData.Note,
	)

	flareStream, err := fc.newFlareStream(flareData.Note, flareData.UploadRequestURL)
	if err != nil {
		fc.slogger.Log(ctx, slog.LevelError,
			"failed to create flare stream, not retrying",
			"err", err,
			"note", flareData.Note,
		)
		return nil
	}

	if err := fc.flarer.RunFlare(context.Background(), fc.knapsack, flareStream); err != nil {
		fc.slogger.Log(ctx, slog.LevelError,
			"failed to run flare, not retrying",
			"err", err,
			"note", flareData.Note,
		)
		return nil
	}

	return nil
}
