package ctxlog

import (
	"context"

	"github.com/go-kit/kit/log"
	"go.opencensus.io/trace"
)

type key int

const loggerKey key = 0

func NewContext(ctx context.Context, logger log.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

func FromContext(ctx context.Context) log.Logger {
	v, ok := ctx.Value(loggerKey).(log.Logger)
	if !ok {
		return log.NewNopLogger()
	}
	span := trace.FromContext(ctx).SpanContext()
	return log.With(
		v,
		"trace_id", span.TraceID.String(),
		"span_id", span.SpanID.String(),
		"trace_is_sampled", span.IsSampled(),
	)
}
