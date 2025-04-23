package exporter

import (
	"context"
	"log/slog"
)

// errorHandler implements the go.opentelemetry.io/otel/internal/global.ErrorHandler interface.
// We use our own error handler instead of the default global one to avoid errors being printed
// to our logs without JSON formatting and without appropriate log level consideration.
type errorHandler struct {
	slogger *slog.Logger
}

func newErrorHandler(slogger *slog.Logger) *errorHandler {
	return &errorHandler{
		slogger: slogger,
	}
}

func (e *errorHandler) Handle(err error) {
	e.slogger.Log(context.TODO(), slog.LevelDebug,
		"tracing error",
		"err", err,
	)
}
