package uninstallconsumer

import (
	"context"
	"io"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/uninstall"
)

const (
	// Identifier for this consumer.
	UninstallSubsystem = "uninstall"
)

type UninstallConsumer struct {
	slogger  *slog.Logger
	knapsack types.Knapsack
}

func New(knapsack types.Knapsack) *UninstallConsumer {
	return &UninstallConsumer{
		slogger:  knapsack.Slogger().With("component", "uninstall_consumer"),
		knapsack: knapsack,
	}
}

func (c *UninstallConsumer) Do(data io.Reader) error {
	c.slogger.Log(context.TODO(), slog.LevelInfo,
		"received request to uninstall",
	)
	uninstall.Uninstall(context.TODO(), c.knapsack, true)
	return nil
}
