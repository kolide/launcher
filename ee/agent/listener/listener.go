package listener

import (
	"log/slog"
	"path/filepath"
	"sync/atomic"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/agent/types"
)

const (
	RootLauncherListenerPipePrefix = "root_launcher_"
)

// launcherListener is a rungroup actor that opens a named pipe and listens on it.
// This allows sufficiently-authenticated processes to communicate with the root
// launcher process.
type launcherListener struct {
	slogger     *slog.Logger
	k           types.Knapsack
	pipePath    string
	interrupt   chan struct{}
	interrupted *atomic.Bool
}

func NewLauncherListener(k types.Knapsack, slogger *slog.Logger, pipeNamePrefix string) *launcherListener {
	return &launcherListener{
		slogger:     slogger.With("component", "launcher_listener", "pipe_prefix", pipeNamePrefix),
		k:           k,
		pipePath:    filepath.Join(k.RootDirectory(), pipeNamePrefix+ulid.New()),
		interrupt:   make(chan struct{}),
		interrupted: &atomic.Bool{},
	}
}

func (e *launcherListener) Execute() error {
	// Wait to shut down whenever launcher shuts down next.
	<-e.interrupt
	return nil
}

func (l *launcherListener) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if l.interrupted.Swap(true) {
		return
	}

	l.interrupt <- struct{}{}
}
