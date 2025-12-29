package listener

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/kolide/launcher/ee/agent/types"
)

const (
	RootLauncherListenerPipePrefix = "root_launcher"
)

// launcherListener is a rungroup actor that opens a named pipe and listens on it.
// This allows sufficiently-authenticated processes to communicate with the root
// launcher process.
type launcherListener struct {
	slogger        *slog.Logger
	k              types.Knapsack
	pipeNamePrefix string
	interrupt      chan struct{}
	interrupted    *atomic.Bool
}

func NewLauncherListener(k types.Knapsack, slogger *slog.Logger, pipeNamePrefix string) *launcherListener {
	return &launcherListener{
		slogger:        slogger.With("component", "launcher_listener", "pipe_name_prefix", pipeNamePrefix),
		k:              k,
		pipeNamePrefix: pipeNamePrefix,
		interrupt:      make(chan struct{}),
		interrupted:    &atomic.Bool{},
	}
}

func (l *launcherListener) Execute() error {
	listener, err := l.initPipe()
	if err != nil {
		l.slogger.Log(context.TODO(), slog.LevelError,
			"unable to init launcher listener",
			"err", err,
		)
		return fmt.Errorf("starting up launcher listener: %w", err)
	}
	l.slogger.Log(context.TODO(), slog.LevelInfo,
		"started up launcher listener",
	)

	// Wait to shut down whenever launcher shuts down next.
	<-l.interrupt
	if err := listener.Close(); err != nil {
		l.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not close listener",
			"err", err,
		)
	}
	return nil
}

func (l *launcherListener) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if l.interrupted.Swap(true) {
		return
	}

	l.interrupt <- struct{}{}
}
