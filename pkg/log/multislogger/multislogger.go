package multislogger

import (
	"context"
	"log/slog"

	slogmulti "github.com/samber/slog-multi"
)

type MultiSlogger struct {
	*slog.Logger
	handlers map[string]MultiSloggerHandler
}

type MultiSloggerHandler struct {
	handler  slog.Handler
	matchers []func(ctx context.Context, r slog.Record) bool
}

func New() *MultiSlogger {
	return &MultiSlogger{
		handlers: make(map[string]MultiSloggerHandler),
		Logger:   slog.New(slogmulti.Router().Handler()),
	}
}

func (m *MultiSlogger) AddReplaceHandler(name string, handler slog.Handler, matchers ...func(ctx context.Context, r slog.Record) bool) *MultiSlogger {
	m.handlers[name] = MultiSloggerHandler{
		handler:  handler,
		matchers: matchers,
	}

	router := slogmulti.Router()
	for _, handler := range m.handlers {
		router = router.Add(handler.handler, handler.matchers...)
	}

	m.Logger = slog.New(
		slogmulti.
			Pipe(slogmulti.NewHandleInlineMiddleware(utcTimeMiddleware)).
			Pipe(slogmulti.NewHandleInlineMiddleware(ctxValuesMiddleWare)).
			Handler(router.Handler()),
	)
	return m
}

func SystemLogMatcher(ctx context.Context, r slog.Record) bool {
	ok := false
	r.Attrs(func(attr slog.Attr) bool {
		if attr.Key == "system_log" && attr.Value.Kind() == slog.KindBool && attr.Value.Bool() == true {
			ok = true

			// stop iteration
			return false
		}
		// continue iteration
		return true
	})

	return ok
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
		v := ctx.Value(key)
		if v == nil {
			continue
		}

		record.AddAttrs(slog.Attr{
			Key:   key,
			Value: slog.AnyValue(v),
		})
	}
	return next(ctx, record)
}
