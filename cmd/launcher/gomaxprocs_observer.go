package main

import (
	"context"
	"log/slog"
	"runtime"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/types"
)

// gomaxprocsObserver watches for changes to the LauncherGoMaxProcs flag
// and applies them at runtime without requiring a restart
type gomaxprocsObserver struct {
	slogger  *slog.Logger
	knapsack types.Knapsack
}

func newGomaxprocsObserver(slogger *slog.Logger, k types.Knapsack) *gomaxprocsObserver {
	return &gomaxprocsObserver{
		slogger:  slogger.With("component", "gomaxprocs_observer"),
		knapsack: k,
	}
}

func (g *gomaxprocsObserver) FlagsChanged(ctx context.Context, flagKeys ...keys.FlagKey) {
	// Check if LauncherGoMaxProcs changed
	for _, key := range flagKeys {
		if key == keys.LauncherGoMaxProcs {
			newLimit := g.knapsack.LauncherGoMaxProcs()
			g.slogger.Log(ctx, slog.LevelInfo,
				"launcher go max procs changed by control server, applying new limit",
				"new_value", newLimit,
			)

			gomaxprocsLimiter(ctx, g.slogger, newLimit)
			return
		}
	}
}

// gomaxprocsLimiter sets a limit on the number of OS threads that can be used at a given time.
func gomaxprocsLimiter(ctx context.Context, slogger *slog.Logger, maxProcs int) {
	cur := runtime.GOMAXPROCS(0)
	if cur <= maxProcs {
		slogger.Log(ctx, slog.LevelInfo,
			"GOMAXPROCS within acceptable range, not changing",
			"current", cur,
			"max", maxProcs,
		)
		return
	}

	slogger.Log(ctx, slog.LevelInfo,
		"limiting GOMAXPROCS",
		"from", cur,
		"to", maxProcs,
	)
	runtime.GOMAXPROCS(maxProcs)
}
