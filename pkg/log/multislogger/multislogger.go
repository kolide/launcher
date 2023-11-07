package multislogger

import (
	"context"
	"io"
	"log/slog"

	slogmulti "github.com/samber/slog-multi"
)

const (
	// KolideSessionIdKey this the also the saml session id
	KolideSessionIdKey = "X-Kolide-Session"
	SpanIdKey          = "span_id"
)

type MultiSlogger struct {
	*slog.Logger
	handlers []slog.Handler
}

// New creates a new multislogger if no handlers are passed in, it will
// create a logger that discards all logs
func New(h ...slog.Handler) *MultiSlogger {
	ms := new(MultiSlogger)

	if len(h) == 0 {
		// if we don't have any handlers passed in, we'll just discard the logs
		// do not add the discard handler to the handlers so it will not be
		// included when a handler is added
		ms.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		return ms
	}

	ms.AddHandler(h...)
	return ms
}

// AddHandler adds a handler to the multislogger
func (m *MultiSlogger) AddHandler(handler ...slog.Handler) {
	m.handlers = append(m.handlers, handler...)

	// we have to rebuild the handler everytime because the slogmulti package we're
	// using doesn't support adding handlers after the Fanout handler has been created
	m.Logger = slog.New(
		slogmulti.
			Pipe(slogmulti.NewHandleInlineMiddleware(utcTimeMiddleware)).
			Pipe(slogmulti.NewHandleInlineMiddleware(ctxValuesMiddleWare)).
			Handler(slogmulti.Fanout(m.handlers...)),
	)
}

func utcTimeMiddleware(ctx context.Context, record slog.Record, next func(context.Context, slog.Record) error) error {
	record.Time = record.Time.UTC()
	return next(ctx, record)
}

var ctxValueKeysToAdd = []string{
	SpanIdKey,
	KolideSessionIdKey,
}

func ctxValuesMiddleWare(ctx context.Context, record slog.Record, next func(context.Context, slog.Record) error) error {
	for _, key := range ctxValueKeysToAdd {
		if v := ctx.Value(key); v != nil {
			record.AddAttrs(slog.Attr{
				Key:   key,
				Value: slog.AnyValue(v),
			})
		}
	}
	return next(ctx, record)
}
