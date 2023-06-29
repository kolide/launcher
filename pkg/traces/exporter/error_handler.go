package exporter

import (
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

// errorHandler implements the go.opentelemetry.io/otel/internal/global.ErrorHandler interface.
// We use our own error handler instead of the default global one to avoid errors being printed
// to our logs without JSON formatting and without appropriate log level consideration.
type errorHandler struct {
	logger log.Logger
}

func newErrorHandler(logger log.Logger) *errorHandler {
	return &errorHandler{
		logger: logger,
	}
}

func (e *errorHandler) Handle(err error) {
	level.Debug(e.logger).Log("msg", "tracing error", "err", err)
}
