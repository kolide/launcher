package types

import (
	"log/slog"
)

type Slogger interface {
	// Logging interface methods
	Slogger() *slog.Logger
	SystemSlogger() *slog.Logger
	AddSlogHandler(handler ...slog.Handler)
}
