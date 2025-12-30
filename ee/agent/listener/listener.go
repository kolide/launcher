package listener

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"os"
	"path/filepath"
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
		interrupt:      make(chan struct{}, 1), // Buffer so that Interrupt can send to this channel and return, even if Execute has already terminated
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

func (l *launcherListener) initPipe() (net.Listener, error) {
	// First, find and remove any existing pipes with the same prefix
	pipeNamePrefixWithPath := filepath.Join(l.k.RootDirectory(), l.pipeNamePrefix)
	matches, err := filepath.Glob(pipeNamePrefixWithPath + "*")
	if err != nil {
		l.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not glob for existing pipes",
			"err", err,
		)
	} else {
		for _, match := range matches {
			if err := os.Remove(match); err != nil {
				l.slogger.Log(context.TODO(), slog.LevelWarn,
					"removing existing pipe",
					"path", match,
					"err", err,
				)
			}
		}
	}

	// Now, create new pipe -- we use a random 4-digit number over ulid to avoid too-long paths.
	pipePath := fmt.Sprintf("%s_%d", pipeNamePrefixWithPath, rand.Intn(10000))
	listener, err := net.Listen("unix", pipePath)
	if err != nil {
		return nil, fmt.Errorf("listening at %s: %w", pipePath, err)
	}

	// Ensure the permissions are set correctly for the socket -- we require root/admin.
	if err := setPipePermissions(pipePath); err != nil {
		listener.Close()
		return nil, fmt.Errorf("chmodding %s: %w", pipePath, err)
	}
	return listener, nil
}

func (l *launcherListener) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if l.interrupted.Swap(true) {
		return
	}

	l.interrupt <- struct{}{}
}
