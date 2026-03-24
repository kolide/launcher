package dedup

import (
	"context"
	"log/slog"
)

// NewMiddleware returns a slog-multi compatible Middleware factory that wraps the
// downstream handler with a dedupHandler. Unlike using Engine.Middleware directly,
// the dedupHandler tracks attributes accumulated via slog.Logger.With() and
// includes them in the dedup hash. This prevents logs from different handler
// chains (e.g. different "component" values) from being incorrectly deduplicated
// together, and ensures that background cleanup emissions use the correct
// downstream handler for each cached entry.
func (d *Engine) NewMiddleware() func(slog.Handler) slog.Handler {
	return func(next slog.Handler) slog.Handler {
		return &dedupHandler{
			engine: d,
			next:   next,
		}
	}
}

// dedupHandler implements slog.Handler, wrapping the Engine's dedup logic while
// tracking accumulated WithAttrs/WithGroup state. This ensures that handler-chain
// attributes (invisible to slog.Record) are included in the dedup hash.
type dedupHandler struct {
	engine *Engine
	next   slog.Handler
	attrs  []slog.Attr
	groups []string
}

var _ slog.Handler = (*dedupHandler)(nil)

func (h *dedupHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *dedupHandler) Handle(ctx context.Context, record slog.Record) error {
	return h.engine.handleRecord(ctx, record, h.attrs, h.groups, h.next.Handle)
}

func (h *dedupHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs), len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	newAttrs = append(newAttrs, attrs...)
	return &dedupHandler{
		engine: h.engine,
		next:   h.next.WithAttrs(attrs),
		attrs:  newAttrs,
		groups: h.groups,
	}
}

func (h *dedupHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	newGroups := make([]string, len(h.groups), len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups = append(newGroups, name)
	return &dedupHandler{
		engine: h.engine,
		next:   h.next.WithGroup(name),
		attrs:  h.attrs,
		groups: newGroups,
	}
}
