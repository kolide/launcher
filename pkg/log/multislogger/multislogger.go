package multislogger

import (
	"context"
	"log/slog"

	slogmulti "github.com/samber/slog-multi"
)

type MultiSlogger struct {
	*slog.Logger
	handlers []multiSloggerHandler
}

type multiSloggerHandler struct {
	handler  slog.Handler
	matchers []func(ctx context.Context, r slog.Record) bool
}

func New() *MultiSlogger {
	return &MultiSlogger{
		Logger: slog.New(slogmulti.Router().Handler()),
	}
}

// AddHandler adds a handler to the multislogger
func (m *MultiSlogger) AddHandler(handler slog.Handler, matchers ...func(ctx context.Context, r slog.Record) bool) *MultiSlogger {
	m.handlers = append(m.handlers, multiSloggerHandler{
		handler:  handler,
		matchers: matchers,
	})

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

// SystemLogMatcher is a matcher that matches records that have the system_log=true attribute.
func SystemLogMatcher(ctx context.Context, r slog.Record) bool {
	isMatch := false
	r.Attrs(func(attr slog.Attr) bool {
		// do the string comparison last since it's more expensive than the kind and bool
		// comparisons which will short circut the if statement if false
		// and not do the compare
		if attr.Value.Kind() == slog.KindBool && attr.Value.Bool() && attr.Key == "system_log" {
			isMatch = true
			// end iteration
			return false
		}

		// continue iteration
		return true
	})

	return isMatch
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
