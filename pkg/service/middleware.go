package service

import (
	"log/slog"

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

func uuidMiddleware(next KolideService) KolideService {
	return uuidmw{next}
}

type uuidmw struct {
	next KolideService
}

// levelForError returns slog.LevelError if err != nil, else slog.LevelDebug
func levelForError(err error) slog.Level {
	if err != nil {
		return slog.LevelError
	}
	return slog.LevelDebug
}
