package service

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/osquery/osquery-go/plugin/distributed"
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

type extractingmw struct {
	knapsack types.Knapsack
	next     KolideService
}

func extractingMiddleware(k types.Knapsack, next KolideService) KolideService {
	return extractingmw{k, next}
}

func (mw extractingmw) extractDenylisted(ctx context.Context, results []distributed.Result) {
	for _, r := range results {
		// We expect that any denylisted queries will have the message "distributed query is denylisted", per
		// https://osquery.readthedocs.io/en/latest/deployment/debugging/#distributed-query-denylisting
		if r.Message != "distributed query is denylisted" {
			continue
		}

		timestamp := strconv.Itoa(int(time.Now().Unix()))

		if err := mw.knapsack.DenylistedQueryAttemptsStore().Set([]byte(timestamp), []byte(r.QueryName)); err != nil {
			mw.knapsack.Slogger().Log(ctx, slog.LevelWarn, "could not store attempt at running denylisted query",
				"err", err,
				"query_name", r.QueryName,
			)
		}
	}
}

// levelForError returns slog.LevelError if err != nil, else slog.LevelDebug
func levelForError(err error) slog.Level {
	if err != nil {
		return slog.LevelError
	}
	return slog.LevelDebug
}
