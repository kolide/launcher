package ctxlog

import (
	"context"
	"io"

	"github.com/go-kit/kit/log"
	"go.opencensus.io/trace"
)

type key int

const (
	loggerKey key = 0
	writerKey key = 1
)

func NewContext(ctx context.Context, logger log.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

func NewContextWithLogFileWriter(ctx context.Context, logger log.Logger, writer io.Writer) context.Context {
	ctx = context.WithValue(ctx, loggerKey, logger)
	return context.WithValue(ctx, writerKey, writer)
}

func FromContext(ctx context.Context) log.Logger {
	v, ok := ctx.Value(loggerKey).(log.Logger)
	if !ok {
		return log.NewNopLogger()
	}
	span := trace.FromContext(ctx).SpanContext()

	// If the span is uninitialized, don't add the 0 values to the
	// logs. They're noise.
	if isTraceUninitialized(span) {
		return v
	}

	return log.With(
		v,
		"trace_id", span.TraceID.String(),
		"span_id", span.SpanID.String(),
		"trace_is_sampled", span.IsSampled(),
	)
}

func FromContextWithLogFileWriter(ctx context.Context) (log.Logger, io.Writer) {
	logger := FromContext(ctx)

	v, ok := ctx.Value(writerKey).(io.Writer)
	if !ok {
		return logger, io.Discard
	}
	return logger, v
}

// isTraceUninitialized returns true when a span is is unconfigured.
func isTraceUninitialized(span trace.SpanContext) bool {
	for _, b := range span.TraceID {
		if b != 0 {
			return false
		}
	}
	return true
}
