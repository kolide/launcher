package tablewrapper

import (
	"context"
	"log/slog"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/kolide/launcher/v2/ee/agent/types"
	"github.com/kolide/launcher/v2/ee/observability"
)

const (
	healthSummarySchemaVersion = 1
	healthReportInterval       = 6 * time.Hour
)

// HealthReporter periodically logs one table_health_summary record with the per-table
// outcomes since the previous record. The log shipper adds device and org identifiers.
// Counts live in memory, so the open window is lost when launcher stops.
type HealthReporter struct {
	knapsack    types.Knapsack
	slogger     *slog.Logger
	recorder    *healthRecorder
	interval    time.Duration // lowered in tests
	interrupt   chan struct{}
	interrupted atomic.Bool
}

func NewHealthReporter(k types.Knapsack) *HealthReporter {
	return &HealthReporter{
		knapsack:  k,
		slogger:   k.Slogger().With("component", "table_health_reporter"),
		recorder:  tableHealth,
		interval:  healthReportInterval,
		interrupt: make(chan struct{}, 1), // buffered so Interrupt never blocks
	}
}

func (r *HealthReporter) Execute() error {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.report(context.Background())
		case <-r.interrupt:
			return nil
		}
	}
}

func (r *HealthReporter) Interrupt(_ error) {
	if r.interrupted.Swap(true) {
		return
	}

	r.interrupt <- struct{}{}
}

// report logs the counts since the previous report. The window is reset even when reporting
// is disabled, so turning it back on never reports counts from the disabled period.
func (r *HealthReporter) report(ctx context.Context) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	tables, windowStart, windowEnd := r.recorder.snapshotAndReset()
	if len(tables) == 0 || !r.knapsack.TableHealthReportingEnabled() {
		return
	}

	enrollmentDetails := r.knapsack.GetEnrollmentDetails()
	r.slogger.Log(ctx, slog.LevelInfo,
		"table_health_summary",
		"schema_version", healthSummarySchemaVersion,
		"window_start", windowStart.UTC().Format(time.RFC3339),
		"window_end", windowEnd.UTC().Format(time.RFC3339),
		"os_name", enrollmentDetails.OSName,
		"os_version", enrollmentDetails.OSVersion,
		"arch", runtime.GOARCH,
		"tables", tables,
	)
}
