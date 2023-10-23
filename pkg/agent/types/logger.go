package types

import (
	"log/slog"
)

type Slogger interface {
	// Logging interface methods
	Slogger() *slog.Logger
	AddLogHandler(handler slog.Handler)
}
