//go:build !windows
// +build !windows

package powereventwatcher

import (
	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/ee/agent/types"
)

type noOpPowerEventWatcher struct {
	interrupt   chan struct{}
	interrupted bool
}

func New(_ types.Knapsack, _ log.Logger) (*noOpPowerEventWatcher, error) {
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
