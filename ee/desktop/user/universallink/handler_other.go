//go:build !darwin
// +build !darwin

package universallink

import (
	"log/slog"
)

// On other OSes, universal link handling is a no-op.
type noopUniversalLinkHandler struct {
	unusedInput chan string
	interrupted bool
	interrupt   chan struct{}
}

func NewUniversalLinkHandler(_ *slog.Logger) (*noopUniversalLinkHandler, chan string) {
	unusedInput := make(chan string, 1)
	return &noopUniversalLinkHandler{
		unusedInput: unusedInput,
		interrupt:   make(chan struct{}),
	}, unusedInput
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
	close(n.unusedInput)
}
