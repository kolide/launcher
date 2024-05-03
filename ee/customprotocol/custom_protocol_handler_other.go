//go:build !darwin
// +build !darwin

package customprotocol

import (
	"log/slog"
)

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
	n.interrupt <- struct{}{}
}
