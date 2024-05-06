//go:build !darwin
// +build !darwin

package customprotocol

import (
	"log/slog"
)

// Currently, we only require custom protocol handling on macOS (in order to
// appropriately support Safari) -- so on other OSes, custom protocol handling
// is a no-op.
type noopCustomProtocolHandler struct {
	interrupted bool
	interrupt   chan struct{}
}

func NewCustomProtocolHandler(_ *slog.Logger) *noopCustomProtocolHandler {
	return &noopCustomProtocolHandler{
		interrupt: make(chan struct{}),
	}
}

func (n *noopCustomProtocolHandler) Execute() error {
	<-n.interrupt
	return nil
}

func (n *noopCustomProtocolHandler) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if n.interrupted {
		return
	}
	n.interrupted = true

	n.interrupt <- struct{}{}
}
