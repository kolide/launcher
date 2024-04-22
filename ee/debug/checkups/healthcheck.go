package checkups

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/restartservice"
)

type (
	healthChecker struct {
		slogger     *slog.Logger
		knapsack    types.Knapsack
		interrupt   chan struct{}
		interrupted bool
		writer      *restartservice.HealthCheckWriter
	}
)

func NewHealthChecker(slogger *slog.Logger, k types.Knapsack, writer *restartservice.HealthCheckWriter) *healthChecker {
	return &healthChecker{
		slogger:   slogger.With("component", "healthchecker"),
		knapsack:  k,
		interrupt: make(chan struct{}, 1),
		writer:    writer,
	}
}

// Run starts a healthchecker routine. The purpose of this is to
// maintain a historical record of launcher health for general debugging
// and for our watchdog service to observe unhealthy states and respond accordingly
func (c *healthChecker) Run() error {
	ticker := time.NewTicker(time.Minute * 30)
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

func (c *healthChecker) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if c.interrupted {
		return
	}

	c.interrupted = true

	c.interrupt <- struct{}{}
}

func (c *healthChecker) Once(ctx context.Context) {
	checkups := checkupsFor(c.knapsack, healthCheckSupported)
	results := make(map[string]Status)
	checkupTime := time.Now().Unix()

	for _, checkup := range checkups {
		checkup.Run(ctx, io.Discard)
		checkupName := normalizeCheckupName(checkup.Name())
		results[checkupName] = checkup.Status()
		// log all data for debugging if Failing
		if checkup.Status() == Failing {
			c.slogger.Log(ctx, slog.LevelWarn,
				"detected health check failure",
				"checkup", checkupName,
				"data", checkup.Data(),
			)
		}
	}

	resultsJson, err := json.Marshal(results)
	if err != nil {
		c.slogger.Log(ctx, slog.LevelWarn,
			"failure encoding health check results",
			"err", err,
		)

		return
	}

	if err = c.writer.AddHealthCheckResult(ctx, checkupTime, resultsJson); err != nil {
		c.slogger.Log(ctx, slog.LevelWarn,
			"failure writing out health check results",
			"err", err,
		)

		return
	}
}

func normalizeCheckupName(name string) string {
	return strings.ReplaceAll(
		strings.ToLower(name),
		" ", "_",
	)
}
