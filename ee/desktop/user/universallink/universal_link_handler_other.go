//go:build !darwin
// +build !darwin

package universallink

import (
	"log/slog"
)

type noopUniversalLinkHandler struct {
	interrupted bool
	interrupt   chan struct{}
}

func NewUniversalLinkHandler(_ *slog.Logger) *noopUniversalLinkHandler {
	return &noopUniversalLinkHandler{
		interrupt: make(chan struct{}),
	}
}

func (n *noopUniversalLinkHandler) Execute() error {
	<-n.interrupt
	return nil
}

func (n *noopUniversalLinkHandler) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if n.interrupted {
		return
	}
	n.interrupted = true

	n.interrupt <- struct{}{}
}
