//go:build darwin || linux

package listener

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"os"
	"path/filepath"
)

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
	if err := os.Chmod(pipePath, 0600); err != nil {
		listener.Close()
		return nil, fmt.Errorf("chmodding %s: %w", pipePath, err)
	}
	return listener, nil
}
