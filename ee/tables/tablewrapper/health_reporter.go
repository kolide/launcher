package tablewrapper

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"runtime"
	"slices"
	"sync/atomic"
	"time"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/v2/ee/agent/flags/keys"
	"github.com/kolide/launcher/v2/ee/agent/types"
	"github.com/kolide/launcher/v2/ee/observability"
)

const (
	healthSummarySchemaVersion = 1
	minHealthReportInterval    = 15 * time.Minute
)

// HealthReporter periodically logs one table_health_summary record with the per-table
// outcomes since the previous record. The log shipper adds device and org identifiers.
type HealthReporter struct {
	knapsack        types.Knapsack
	slogger         *slog.Logger
	recorder        *healthRecorder
	minInterval     time.Duration // floor for the reporting interval; lowered in tests
	intervalChanged chan struct{}
	interrupt       chan struct{}
	interrupted     atomic.Bool
}

func NewHealthReporter(k types.Knapsack) *HealthReporter {
	r := &HealthReporter{
		knapsack:    k,
		slogger:     k.Slogger().With("component", "table_health_reporter"),
		recorder:    tableHealth,
		minInterval: minHealthReportInterval,
		// Both channels are buffered so that FlagsChanged and Interrupt never block.
		intervalChanged: make(chan struct{}, 1),
		interrupt:       make(chan struct{}, 1),
	}
	k.RegisterChangeObserver(r, keys.TableHealthReportInterval)

	return r
}

// Execute reports once per jittered interval until interrupted. When the interval flag
// changes, the deadline for the open window is recomputed from the window's start, so a
// shorter interval applies right away and no window is lost or reported twice.
func (r *HealthReporter) Execute() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	interval := r.currentInterval()
	timer := time.NewTimer(r.untilDeadline(interval))
	defer timer.Stop()
	if interval == 0 {
		timer.Stop()
	}

	for {
		select {
		case <-timer.C:
			r.report(ctx)
			timer.Reset(jitter(interval))
		case <-r.intervalChanged:
			newInterval := r.currentInterval()
			if newInterval == interval {
				continue
			}
			wasEnabled := interval > 0
			interval = newInterval

			r.slogger.Log(ctx, slog.LevelInfo,
				"table health report interval changed",
				"interval", interval.String(),
			)

			if interval == 0 {
				timer.Stop()
				continue
			}
			if !wasEnabled {
				// Drop counts gathered while reporting was disabled, so the first window starts now.
				r.recorder.snapshotAndReset()
			}
			// A deadline that has already passed fires immediately.
			timer.Reset(r.untilDeadline(interval))
		case <-r.interrupt:
			if interval > 0 {
				// Best effort: the log shipper may already be draining.
				r.report(ctx)
			}
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

// FlagsChanged satisfies types.FlagsChangeObserver. It only signals Execute, which re-reads
// the flag; signals that arrive before Execute handles the previous one are coalesced.
func (r *HealthReporter) FlagsChanged(_ context.Context, flagKeys ...keys.FlagKey) {
	if !slices.Contains(flagKeys, keys.TableHealthReportInterval) {
		return
	}

	select {
	case r.intervalChanged <- struct{}{}:
	default:
	}
}

// currentInterval returns the configured interval raised to the floor, or 0 when disabled.
func (r *HealthReporter) currentInterval() time.Duration {
	interval := r.knapsack.TableHealthReportInterval()
	if interval <= 0 {
		return 0
	}
	return max(interval, r.minInterval)
}

// untilDeadline returns how long until the open window should be reported.
func (r *HealthReporter) untilDeadline(interval time.Duration) time.Duration {
	return time.Until(r.recorder.windowStartedAt().Add(jitter(interval)))
}

func (r *HealthReporter) report(ctx context.Context) {
	ctx, span := observability.StartSpan(ctx)
	defer span.End()

	tables, windowStart, windowEnd := r.recorder.snapshotAndReset()
	if len(tables) == 0 {
		return
	}

	enrollmentDetails := r.knapsack.GetEnrollmentDetails()
	r.slogger.Log(ctx, slog.LevelInfo,
		"table_health_summary",
		"schema_version", healthSummarySchemaVersion,
		"record_id", ulid.New(),
		"window_start", windowStart.UTC().Format(time.RFC3339),
		"window_end", windowEnd.UTC().Format(time.RFC3339),
		"os_name", enrollmentDetails.OSName,
		"os_version", enrollmentDetails.OSVersion,
		"arch", runtime.GOARCH,
		"tables", tables,
	)
}

// jitter returns d spread by ±10%, so devices don't all report at once.
func jitter(d time.Duration) time.Duration {
	spread := d / 10
	return d - spread + rand.N(2*spread+1)
}
