package main

import (
	"context"
	"log/slog"
	"runtime"
	"sync"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/types"
)

var (
	// initialGOMAXPROCS captures the starting GOMAXPROCS value at process startup.
	// This serves as the upper bound when resetting or increasing GOMAXPROCS.
	initialGOMAXPROCS     int
	initialGOMAXPROCSOnce sync.Once
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
	// Capture the initial GOMAXPROCS value once at first call
	initialGOMAXPROCSOnce.Do(func() {
		initialGOMAXPROCS = runtime.GOMAXPROCS(0)
		slogger.Log(ctx, slog.LevelDebug,
			"captured initial GOMAXPROCS",
			"value", initialGOMAXPROCS,
		)
	})

	current := runtime.GOMAXPROCS(0)

	// If maxProcs is 0 or exceeds the initial value, reset to initial
	if maxProcs <= 0 || maxProcs > initialGOMAXPROCS {
		if current != initialGOMAXPROCS {
			slogger.Log(ctx, slog.LevelInfo,
				"resetting GOMAXPROCS to initial value",
				"from", current,
				"to", initialGOMAXPROCS,
			)
			runtime.GOMAXPROCS(initialGOMAXPROCS)
		}
		return
	}

	// Apply the limit if it differs from current
	if current != maxProcs {
		slogger.Log(ctx, slog.LevelInfo,
			"changing GOMAXPROCS",
			"from", current,
			"to", maxProcs,
		)
		runtime.GOMAXPROCS(maxProcs)
	}
}
