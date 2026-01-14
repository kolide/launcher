//go:build !windows

package multislogger

import (
	"io"
	"log/slog"
	"os"
)

func SystemSlogger() (*MultiSlogger, io.Closer, error) {
	return defaultSystemSlogger(), io.NopCloser(nil), nil
}

func defaultSystemSlogger() *MultiSlogger {
	return New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}
