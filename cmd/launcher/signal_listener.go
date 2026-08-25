package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
)

// listens for interrupts
type signalListener struct {
	sigChannel  chan os.Signal
	cancel      context.CancelFunc
	slogger     *slog.Logger
	interrupt   chan struct{}
	interrupted atomic.Bool
}

func newSignalListener(sigChannel chan os.Signal, cancel context.CancelFunc, slogger *slog.Logger) *signalListener {
	signal.Notify(sigChannel, os.Interrupt, syscall.SIGTERM)
	return &signalListener{
		sigChannel: sigChannel,
		cancel:     cancel,
		slogger:    slogger.With("component", "signal_listener"),
		interrupt:  make(chan struct{}, 1),
	}
}

func (s *signalListener) Execute() error {
	select {
	case sig := <-s.sigChannel:
		s.slogger.Log(context.TODO(), slog.LevelInfo,
			"beginning shutdown via signal",
			"signal_received", sig,
		)
	case <-s.interrupt:
		// Rungroup shutdown
	}

	return nil
}

func (s *signalListener) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if s.interrupted.Swap(true) {
		return
	}

	// tell sender in `os/signal` package to stop sending on `s.sigChannel`
	// to avoid panics for sending on a closed channel
	signal.Stop(s.sigChannel)
	close(s.interrupt)
	s.cancel()
}
