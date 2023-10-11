package uninstallconsumer

import (
	"io"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/launcher/uninstall"
)

const (
	// Identifier for this consumer.
	UninstallSubsystem = "uninstall"
)

type UninstallConsumer struct {
	logger   log.Logger
	knapsack types.Knapsack
}

func New(logger log.Logger, knapsack types.Knapsack) *UninstallConsumer {
	return &UninstallConsumer{
		logger:   logger,
		knapsack: knapsack,
	}
}

func (c *UninstallConsumer) Do(data io.Reader) error {
	uninstall.Uninstall(c.logger, c.knapsack)
	return nil
}
