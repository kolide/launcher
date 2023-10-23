package multislogger

import (
	"log/slog"

	slogmulti "github.com/samber/slog-multi"
)

type MultiSlogger struct {
	*slog.Logger
	handlers []slog.Handler
}

func New(handler slog.Handler) *MultiSlogger {
	multislogger := &MultiSlogger{}
	multislogger.AddHandler(handler)
	return multislogger
}

func (m *MultiSlogger) AddHandler(handler slog.Handler) *MultiSlogger {
	m.handlers = append(m.handlers, handler)
	m.Logger = slog.New(slogmulti.Fanout(m.handlers...))
	return m
}
