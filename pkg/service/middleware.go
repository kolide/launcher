package service

import (
	"context"
	"log/slog"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/types"
)

type Middleware func(KolideService) KolideService

func LoggingMiddleware(k types.Knapsack) Middleware {
	return func(next KolideService) KolideService {
		return logmw{k, next}
	}
}

type logmw struct {
	knapsack types.Knapsack
	next     KolideService
}

// FlagsChanged passes through to the underlying client, for it to handle changes to the server URL.
func (mw logmw) FlagsChanged(ctx context.Context, flagKeys ...keys.FlagKey) {
	mw.next.FlagsChanged(ctx, flagKeys...)
}

func uuidMiddleware(next KolideService) KolideService {
	return uuidmw{next}
}

type uuidmw struct {
	next KolideService
}

// FlagsChanged passes through to the underlying client, for it to handle changes to the server URL.
func (mw uuidmw) FlagsChanged(ctx context.Context, flagKeys ...keys.FlagKey) {
	mw.next.FlagsChanged(ctx, flagKeys...)
}

// levelForError returns slog.LevelWarn if err != nil, else slog.LevelDebug
func levelForError(err error) slog.Level {
	if err != nil {
		return slog.LevelWarn
	}
	return slog.LevelDebug
}
