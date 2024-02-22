package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

// listens for interrupts
type signalListener struct {
	sigChannel  chan os.Signal
	cancel      context.CancelFunc
	slogger     *slog.Logger
	interrupted bool
}

func newSignalListener(sigChannel chan os.Signal, cancel context.CancelFunc, slogger *slog.Logger) *signalListener {
	return &signalListener{
		sigChannel: sigChannel,
		cancel:     cancel,
		slogger:    slogger.With("component", "signal_listener"),
	}
}

func (s *signalListener) Execute() error {
	signal.Notify(s.sigChannel, os.Interrupt, syscall.SIGTERM)
	sig := <-s.sigChannel
	s.slogger.Log(context.TODO(), slog.LevelInfo,
		"beginning shutdown via signal",
		"signal_received", sig,
	)
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
