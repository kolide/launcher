package ctxlog

import (
	"context"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"go.opencensus.io/trace"
)

type key int

const (
	loggerKey  key = 0
	sloggerKey key = 1
)

func NewContext(ctx context.Context, logger log.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

func NewContextWithMultislogger(ctx context.Context, slogger *multislogger.MultiSlogger) context.Context {
	return context.WithValue(ctx, sloggerKey, slogger)
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

func FromContextWithSlogger(ctx context.Context) *multislogger.MultiSlogger {
	v, ok := ctx.Value(sloggerKey).(*multislogger.MultiSlogger)
	if !ok {
		return nil
	}
	return v
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
