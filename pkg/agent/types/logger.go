package types

import (
	"log/slog"
)

type Logger interface {
	// Logging interface methods
	Logger() *slog.Logger
	AddLogHandler(handler slog.Handler)
}
