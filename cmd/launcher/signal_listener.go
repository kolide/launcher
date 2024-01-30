package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

// listens for interrupts
type signalListener struct {
	sigChannel  chan os.Signal
	cancel      context.CancelFunc
	logger      log.Logger
	interrupted bool
}

func newSignalListener(sigChannel chan os.Signal, cancel context.CancelFunc, logger log.Logger) *signalListener {
	return &signalListener{
		sigChannel: sigChannel,
		cancel:     cancel,
		logger:     log.With(logger, "component", "signal_listener"),
	}
}

func (s *signalListener) Execute() error {
	signal.Notify(s.sigChannel, os.Interrupt, syscall.SIGTERM)
	sig := <-s.sigChannel
	level.Info(s.logger).Log("msg", "beginning shutdown via signal", "signal_received", sig)
	return nil
}

func (s *signalListener) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if s.interrupted {
		return
	}
	s.interrupted = true
	s.cancel()
	close(s.sigChannel)
}
