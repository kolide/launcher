package multislogger

import (
	"context"
	"log/slog"

	"github.com/kolide/kit/ulid"
	slogmulti "github.com/samber/slog-multi"
)

type contextKey string

func (c contextKey) String() string {
	return string(c)
}

const (
	// KolideSessionIdKey this the also the saml session id
	KolideSessionIdKey contextKey = "kolide_session_id"
	SpanIdKey          contextKey = "span_id"
	TraceIdKey         contextKey = "trace_id"
	TraceSampledKey    contextKey = "trace_sampled"
)

// ctxValueKeysToAdd is a list of context keys that will be
// added as log attributes
var ctxValueKeysToAdd = []contextKey{
	SpanIdKey,
	TraceIdKey,
	KolideSessionIdKey,
	TraceSampledKey,
}

type MultiSlogger struct {
	*slog.Logger
	handlers      []slog.Handler
	launcherRunId string
}

// New creates a new multislogger if no handlers are passed in, it will
// create a logger that discards all logs
func New(h ...slog.Handler) *MultiSlogger {
	ms := &MultiSlogger{
		// setting to fanout with no handlers is noop
		Logger:        slog.New(slogmulti.Fanout()),
		launcherRunId: ulid.New(),
	}

	ms.AddHandler(h...)
	return ms
}

// NewNopLogger returns a slogger with no handlers, discarding all logs.
func NewNopLogger() *slog.Logger {
	return New().Logger
}

// AddHandler adds a handler to the multislogger, this creates a branch new
// slog.Logger under the the hood, and overwrites old Logger memory address,
// this means any attributes added with Logger.With will be lost
func (m *MultiSlogger) AddHandler(handler ...slog.Handler) {
	m.handlers = append(m.handlers, handler...)

	// we have to rebuild the handler everytime because the slogmulti package we're
	// using doesn't support adding handlers after the Fanout handler has been created
	*m.Logger = *slog.New(
		slogmulti.
			Pipe(slogmulti.NewHandleInlineMiddleware(utcTimeMiddleware)).
			Pipe(slogmulti.NewHandleInlineMiddleware(ctxValuesMiddleWare)).
			Handler(slogmulti.Fanout(m.handlers...)),
	).With("launcher_run_id", m.launcherRunId)
}

func utcTimeMiddleware(ctx context.Context, record slog.Record, next func(context.Context, slog.Record) error) error {
	record.Time = record.Time.UTC()
	return next(ctx, record)
}

func ctxValuesMiddleWare(ctx context.Context, record slog.Record, next func(context.Context, slog.Record) error) error {
	for _, key := range ctxValueKeysToAdd {
		if v := ctx.Value(key); v != nil {
			record.AddAttrs(slog.Attr{
				Key:   key.String(),
				Value: slog.AnyValue(v),
			})
		}
	}

	return next(ctx, record)
}
