package uninstallconsumer

import (
	"context"
	"io"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/uninstall"
)

const (
	// Identifier for this consumer.
	UninstallSubsystem = "uninstall"
)

type UninstallConsumer struct {
	knapsack types.Knapsack
}

func New(knapsack types.Knapsack) *UninstallConsumer {
	return &UninstallConsumer{
		knapsack: knapsack,
	}
}

func (c *UninstallConsumer) Do(data io.Reader) error {
	uninstall.Uninstall(context.TODO(), c.knapsack, true)
	return nil
}
