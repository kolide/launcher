//go:build !windows
// +build !windows

package powereventwatcher

import (
	"context"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/traces"
)

type noOpPowerEventWatcher struct {
	interrupt   chan struct{}
	interrupted bool
}

func New(ctx context.Context, _ types.Knapsack, _ *slog.Logger) (*noOpPowerEventWatcher, error) {
	_, span := traces.StartSpan(ctx)
	defer span.End()

	return &noOpPowerEventWatcher{
		interrupt: make(chan struct{}),
	}, nil
}

func (n *noOpPowerEventWatcher) Execute() error {
	<-n.interrupt
	return nil
}

func (n *noOpPowerEventWatcher) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if n.interrupted {
		return
	}

	n.interrupted = true

	n.interrupt <- struct{}{}
}
