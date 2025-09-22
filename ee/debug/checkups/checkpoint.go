package checkups

import (
	"context"
	"io"
	"log/slog"
	"strings"
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
	if c.interrupted.Swap(true) {
		return
	}

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
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	checkups := checkupsFor(c.knapsack, logSupported)

	warningCheckups := make([]string, 0)
	failingCheckups := make([]string, 0)
	passingCheckups := make([]string, 0)
	erroringCheckups := make([]string, 0)

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
			warningCheckups = append(warningCheckups, checkup.Name())
		case Failing:
			failingCheckups = append(failingCheckups, checkup.Name())
		case Passing:
			passingCheckups = append(passingCheckups, checkup.Name())
		case Erroring:
			erroringCheckups = append(erroringCheckups, checkup.Name())
		case Informational, Unknown:
			// Nothing to do here
		}
	}

	// Compute score from warning, passing, and failing counts
	passingCount := float64(len(passingCheckups))
	warningCount := float64(len(warningCheckups))
	scoredCheckups := float64(len(warningCheckups) + len(failingCheckups) + len(passingCheckups))
	score := ((passingCount + (warningCount / 2)) / scoredCheckups) * 100
	observability.CheckupScoreGauge.Record(ctx, score)

	logLevel := slog.LevelDebug
	if score < 100 {
		logLevel = slog.LevelWarn
	}
	c.slogger.Log(ctx, logLevel,
		"computed checkup score",
		"score", score,
		"failing_checkups", strings.Join(failingCheckups, ","),
		"warning_checkups", strings.Join(warningCheckups, ","),
		"passing_checkups", strings.Join(passingCheckups, ","),
		"erroring_checkups", strings.Join(erroringCheckups, ","),
		"total_scored_checkups", scoredCheckups,
	)

	// Record number of errors separately
	observability.CheckupErrorCounter.Add(ctx, int64(len(erroringCheckups)))
}
