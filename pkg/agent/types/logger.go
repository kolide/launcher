package types

import (
	"context"
	"log/slog"
)

type Slogger interface {
	// Logging interface methods
	Slogger() *slog.Logger
	AddReplaceSlogHandler(name string, handler slog.Handler, matchers ...func(ctx context.Context, r slog.Record) bool)
}
