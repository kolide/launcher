package flareconsumer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/debug/checkups"
)

const (
	// Identifier for this consumer.
	FlareSubsystem = "flare"
)

type FlareConsumer struct {
	flarer   flarer
	shipper  shipper
	knapsack types.Knapsack
}

type flarer interface {
	RunFlare(ctx context.Context, k types.Knapsack, flareStream io.Writer, runtimeEnvironment checkups.RuntimeEnvironmentType) error
}

type shipper interface {
	Ship(log log.Logger, k types.Knapsack, note string, flareStream io.Reader) error
}

func New(knapsack types.Knapsack, flarer flarer, shipper shipper) *FlareConsumer {
	return &FlareConsumer{
		flarer:   flarer,
		knapsack: knapsack,
		shipper:  shipper,
	}
}

func (fc *FlareConsumer) Do(data io.Reader) error {
	if fc.flarer == nil {
		return errors.New("flarer is nil")
	}

	if fc.shipper == nil {
		return errors.New("shipper is nil")
	}

	flareData := struct {
		Note string `json:"note"`
	}{}

	if err := json.NewDecoder(data).Decode(&flareData); err != nil {
		return fmt.Errorf("failed to decode key-value json: %w", err)
	}

	buf := &bytes.Buffer{}

	if err := fc.flarer.RunFlare(context.Background(), fc.knapsack, buf, checkups.InSituEnvironment); err != nil {
		return err
	}

	return fc.shipper.Ship(log.NewNopLogger(), fc.knapsack, flareData.Note, buf)
}
