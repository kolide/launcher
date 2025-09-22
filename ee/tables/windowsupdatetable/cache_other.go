//go:build !windows
// +build !windows

package windowsupdatetable

import (
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
)

type noOpWindowsUpdatesCacher struct {
	interrupt   chan struct{}
	interrupted atomic.Bool
}

func NewWindowsUpdatesCacher(_ types.Flags, _ types.GetterSetter, _ time.Duration, _ *slog.Logger) *noOpWindowsUpdatesCacher {
	return &noOpWindowsUpdatesCacher{
		interrupt: make(chan struct{}),
	}
}

func (n *noOpWindowsUpdatesCacher) Execute() error {
	<-n.interrupt
	return nil
}

func (n *noOpWindowsUpdatesCacher) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if n.interrupted.Swap(true) {
		return
	}

	n.interrupt <- struct{}{}
}
