package checkups

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
)

type (
	logCheckPointer struct {
		slogger     *slog.Logger
		knapsack    types.Knapsack
		interrupt   chan struct{}
		interrupted atomic.Bool
	}
)

func NewCheckupLogger(slogger *slog.Logger, k types.Knapsack) *logCheckPointer {
	return &logCheckPointer{
		slogger:   slogger.With("component", "log_checkpoint"),
		knapsack:  k,
		interrupt: make(chan struct{}, 1),
	}
}

// Run starts a log checkpoint routine. The purpose of this is to
// ensure we get good debugging information in the logs.
func (c *logCheckPointer) Run() error {
	ticker := time.NewTicker(time.Minute * 60)
	defer ticker.Stop()

	for {
		c.Once(context.TODO())

		select {
		case <-ticker.C:
			continue
		case <-c.interrupt:
			c.slogger.Log(context.TODO(), slog.LevelDebug,
				"interrupt received, exiting execute loop",
			)
			return nil
		}
	}
}

func (c *logCheckPointer) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if c.interrupted.Load() {
		return
	}

	c.interrupted.Store(true)

	c.interrupt <- struct{}{}
}

func (c *logCheckPointer) Once(ctx context.Context) {
	checkups := checkupsFor(c.knapsack, logSupported)

	for _, checkup := range checkups {
		checkup.Run(ctx, io.Discard)

		c.slogger.Log(ctx, slog.LevelDebug,
			"ran checkup",
			"checkup", checkup.Name(),
			"summary", checkup.Summary(),
			"data", checkup.Data(),
			"status", checkup.Status(),
		)
	}
}
