package checkups

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/observability"
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
	// We want to wait slightly to allow some stats like CPU percentage to settle.
	select {
	case <-c.interrupt:
		c.slogger.Log(context.TODO(), slog.LevelDebug,
			"received external interrupt during initial delay, stopping",
		)
		return nil
	case <-time.After(1 * time.Minute):
		break
	}

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

// LogCheckupsOnStartup is intended to be called on launcher startup; it runs and logs all
// checkups that are appropriate for launcher startup to get that data into the logs as soon
// as possible. Notably, it does not run the performance checkup, because launcher has not been
// running long enough for that data to be meaningful.
func (c *logCheckPointer) LogCheckupsOnStartup(ctx context.Context) {
	checkups := checkupsFor(c.knapsack, startupLogSupported)
	for _, checkup := range checkups {
		checkup.Run(ctx, io.Discard)

		c.slogger.Log(ctx, slog.LevelDebug,
			"ran checkup on startup",
			"checkup", checkup.Name(),
			"summary", checkup.Summary(),
			"data", checkup.Data(),
			"status", checkup.Status(),
		)
	}
}

// Once runs all log-supported checkups. It logs the status of each, and additionally calculates a score
// based on those statuses; it logs the score and reports it as a metric. Once allows us to see a snapshot
// of launcher health.
func (c *logCheckPointer) Once(ctx context.Context) {
	checkups := checkupsFor(c.knapsack, logSupported)

	warningCount := 0.0
	failingCount := 0.0
	passingCount := 0.0
	erroringCount := 0.0

	for _, checkup := range checkups {
		checkup.Run(ctx, io.Discard)

		c.slogger.Log(ctx, slog.LevelDebug,
			"ran checkup",
			"checkup", checkup.Name(),
			"summary", checkup.Summary(),
			"data", checkup.Data(),
			"status", checkup.Status(),
		)

		switch checkup.Status() {
		case Warning:
			warningCount += 1
		case Failing:
			failingCount += 1
		case Passing:
			passingCount += 1
		case Erroring:
			erroringCount += 1
		case Informational, Unknown:
			// Nothing to do here
		}
	}

	// Compute score from warning, passing, and failing counts
	scoredCheckups := warningCount + failingCount + passingCount
	score := ((passingCount + (warningCount / 2)) / scoredCheckups) * 100
	observability.CheckupScoreGauge.Record(ctx, score)

	c.slogger.Log(ctx, slog.LevelDebug,
		"computed checkup score",
		"score", score,
		"failing_count", failingCount,
		"warning_count", warningCount,
		"passing_count", passingCount,
		"total_scored_checkups", scoredCheckups,
	)

	// Record number of errors separately
	observability.CheckupErrorCounter.Add(ctx, int64(erroringCount))
}
