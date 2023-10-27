package multislogger

import (
	"context"
	"log/slog"

	slogmulti "github.com/samber/slog-multi"
)

type MultiSlogger struct {
	*slog.Logger
	handlers []slog.Handler
}

// AddHandler adds a handler to the multislogger
func (m *MultiSlogger) AddHandler(handler slog.Handler) *MultiSlogger {
	m.handlers = append(m.handlers, handler)

	m.Logger = slog.New(
		slogmulti.
			Pipe(slogmulti.NewHandleInlineMiddleware(utcTimeMiddleware)).
			Pipe(slogmulti.NewHandleInlineMiddleware(ctxValuesMiddleWare)).
			// we have to rebuild the handler everytime because the slogmulti package we're
			// using doesn't support adding handlers after the Fanout handler has been created
			Handler(slogmulti.Fanout(m.handlers...)),
	)

	return m
}

func utcTimeMiddleware(ctx context.Context, record slog.Record, next func(context.Context, slog.Record) error) error {
	record.Time = record.Time.UTC()
	return next(ctx, record)
}

var ctxValueKeysToAdd = []string{
	"span_id",
	"saml_session_id",
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
