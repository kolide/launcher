package flareconsumer

import (
	"bytes"
	"context"
	"errors"
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
	Ship(log log.Logger, k types.Knapsack, flareStream io.Reader) error
}

func New(knapsack types.Knapsack, flarer flarer, shipper shipper) *FlareConsumer {
	return &FlareConsumer{
		flarer:   flarer,
		knapsack: knapsack,
		shipper:  shipper,
	}
}

func (fc *FlareConsumer) Do(_ io.Reader) error {
	if fc.flarer == nil {
		return errors.New("flarer is nil")
	}

	if fc.shipper == nil {
		return errors.New("shipper is nil")
	}

	buf := &bytes.Buffer{}

	if err := fc.flarer.RunFlare(context.Background(), fc.knapsack, buf, checkups.InSituEnvironment); err != nil {
		return err
	}

	return fc.shipper.Ship(log.NewNopLogger(), fc.knapsack, buf)
}
