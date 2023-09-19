package flareconsumer

import (
	"context"
	"io"

	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/debug/checkups"
)

type FlareRunner struct{}

func (f *FlareRunner) RunFlare(ctx context.Context, k types.Knapsack, flareStream io.Writer, runtimeEnvironment checkups.RuntimeEnvironmentType) error {
	return checkups.RunFlare(ctx, k, flareStream, runtimeEnvironment)
}
