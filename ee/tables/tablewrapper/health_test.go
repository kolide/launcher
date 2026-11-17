package tablewrapper

import (
	"context"
	"encoding/json"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kolide/launcher/v2/ee/agent/flags/keys"
	"github.com/kolide/launcher/v2/ee/agent/types"
	typesmocks "github.com/kolide/launcher/v2/ee/agent/types/mocks"
	"github.com/kolide/launcher/v2/ee/gowrapper"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/kolide/launcher/v2/pkg/threadsafebuffer"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// countsFor returns the current counts for one table without resetting the window.
func (h *healthRecorder) countsFor(tableName string) TableCounts {
	h.lock.Lock()
	defer h.lock.Unlock()

	if s, ok := h.tables[tableName]; ok {
		return s.read()
	}
	return TableCounts{Name: tableName}
}

// since returns the counts added after an earlier reading. The process-wide recorder keeps
// its counts across runs of a test (-count=N), so tests against it compare deltas.
// TotalMs and MaxMs are left out: they depend on how long the calls really took.
func (c TableCounts) since(earlier TableCounts) TableCounts {
	return TableCounts{
		Name:     c.Name,
		Query:    c.Query - earlier.Query,
		Error:    c.Error - earlier.Error,
		Empty:    c.Empty - earlier.Empty,
		Timeout:  c.Timeout - earlier.Timeout,
		NoWorker: c.NoWorker - earlier.NoWorker,
	}
}

func TestHealthRecorder_record(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		outcomes []queryOutcome
		expected TableCounts
	}{
		{name: "success", outcomes: []queryOutcome{outcomeSuccess}, expected: TableCounts{Query: 1}},
		{name: "empty", outcomes: []queryOutcome{outcomeEmpty}, expected: TableCounts{Query: 1, Empty: 1}},
		{name: "error", outcomes: []queryOutcome{outcomeError}, expected: TableCounts{Query: 1, Error: 1}},
		{name: "timeout is a query but not an error", outcomes: []queryOutcome{outcomeTimeout}, expected: TableCounts{Query: 1, Timeout: 1}},
		{name: "no worker is not a query", outcomes: []queryOutcome{outcomeNoWorker}, expected: TableCounts{NoWorker: 1}},
		{
			name:     "mixed",
			outcomes: []queryOutcome{outcomeSuccess, outcomeEmpty, outcomeError, outcomeTimeout, outcomeNoWorker, outcomeError},
			expected: TableCounts{Query: 5, Error: 2, Empty: 1, Timeout: 1, NoWorker: 1},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHealthRecorder()
			for _, o := range tt.outcomes {
				h.record("test_table", o, 0)
			}

			tt.expected.Name = "test_table"
			require.Equal(t, tt.expected, h.countsFor("test_table"))
		})
	}
}

func TestHealthRecorder_recordDurations(t *testing.T) {
	t.Parallel()

	type call struct {
		outcome queryOutcome
		elapsed time.Duration
	}

	for _, tt := range []struct {
		name            string
		calls           []call
		expectedTotalMs int64
		expectedMaxMs   int64
	}{
		{
			name:            "single call",
			calls:           []call{{outcomeSuccess, 40 * time.Millisecond}},
			expectedTotalMs: 40,
			expectedMaxMs:   40,
		},
		{
			name: "every outcome that counts as a query adds its time",
			calls: []call{
				{outcomeSuccess, 10 * time.Millisecond},
				{outcomeEmpty, 20 * time.Millisecond},
				{outcomeError, 30 * time.Millisecond},
				{outcomeTimeout, 4 * time.Minute},
			},
			expectedTotalMs: 240_060,
			expectedMaxMs:   240_000,
		},
		{
			name:            "no worker adds no time",
			calls:           []call{{outcomeNoWorker, time.Second}, {outcomeSuccess, 5 * time.Millisecond}},
			expectedTotalMs: 5,
			expectedMaxMs:   5,
		},
		{
			name: "sub-millisecond calls add up before rounding",
			calls: []call{
				{outcomeSuccess, 400 * time.Microsecond},
				{outcomeSuccess, 400 * time.Microsecond},
				{outcomeSuccess, 400 * time.Microsecond},
				{outcomeSuccess, 400 * time.Microsecond},
				{outcomeSuccess, 400 * time.Microsecond},
			},
			expectedTotalMs: 2,
			expectedMaxMs:   0,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHealthRecorder()
			for _, c := range tt.calls {
				h.record("test_table", c.outcome, c.elapsed)
			}

			counts, _, _ := h.snapshotAndReset()
			require.Len(t, counts, 1)
			require.Equal(t, tt.expectedTotalMs, counts[0].TotalMs, "total_ms")
			require.Equal(t, tt.expectedMaxMs, counts[0].MaxMs, "max_ms")

			// Durations start over with the next window
			h.record("test_table", outcomeSuccess, time.Millisecond)
			counts, _, _ = h.snapshotAndReset()
			require.Len(t, counts, 1)
			require.Equal(t, int64(1), counts[0].TotalMs, "total_ms after reset")
			require.Equal(t, int64(1), counts[0].MaxMs, "max_ms after reset")
		})
	}
}

func TestHealthRecorder_snapshotAndReset(t *testing.T) {
	t.Parallel()

	h := newHealthRecorder()
	h.record("b_table", outcomeSuccess, 0)
	h.record("b_table", outcomeEmpty, 0)
	h.record("b_table", outcomeError, 0)
	h.record("b_table", outcomeTimeout, 0)
	h.record("a_table", outcomeNoWorker, 0)

	counts, windowStart, windowEnd := h.snapshotAndReset()
	require.Equal(t, []TableCounts{
		{Name: "a_table", NoWorker: 1},
		{Name: "b_table", Query: 4, Error: 1, Empty: 1, Timeout: 1},
	}, counts, "only tables with counts are listed, sorted by name")
	require.False(t, windowEnd.Before(windowStart))

	// The next window starts where the last one ended, with no counts
	require.Equal(t, windowEnd, h.windowStartedAt())
	counts, nextWindowStart, _ := h.snapshotAndReset()
	require.Empty(t, counts)
	require.Equal(t, windowEnd, nextWindowStart)
}

func TestHealthRecorder_concurrentRecordsAndSnapshots(t *testing.T) {
	t.Parallel()

	h := newHealthRecorder()
	var total atomic.Int64
	snapshot := func() {
		counts, _, _ := h.snapshotAndReset()
		for _, c := range counts {
			total.Add(c.Error)
		}
	}

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			for range 100 {
				h.record("concurrent_table", outcomeError, 0)
			}
		})
	}
	for range 5 {
		wg.Go(snapshot)
	}
	wg.Wait()
	snapshot()

	// Every record lands in exactly one snapshot
	require.Equal(t, int64(5000), total.Load())
}

func TestGenerate_recordsTimeoutAndNoWorker(t *testing.T) {
	t.Parallel()

	mockFlags := typesmocks.NewFlags(t)
	mockFlags.On("TableGenerateTimeout").Return(4 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, keys.TableGenerateTimeout).Return()

	tableName := "test_health_timeout_no_worker"
	before := tableHealth.countsFor(tableName)
	wt := newWrappedTable(mockFlags, multislogger.NewNopLogger(), tableName, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}, WithTableGenerateTimeout(10*time.Millisecond))

	_, err := wt.generate(t.Context(), table.QueryContext{})
	require.Error(t, err)

	// Occupy every worker so the next call is turned away. The timed-out call's worker is
	// released shortly after generate returns, so wait for it.
	require.Eventually(t, func() bool { return wt.workers.TryAcquire(numWorkers) }, 5*time.Second, time.Millisecond)
	_, err = wt.generate(t.Context(), table.QueryContext{})
	require.ErrorContains(t, err, "no workers available")
	wt.workers.Release(numWorkers)

	after := tableHealth.countsFor(tableName)
	require.Equal(t, TableCounts{Name: tableName, Query: 1, Timeout: 1, NoWorker: 1}, after.since(before))

	// The timed-out call counts for at least as long as we waited for it
	require.GreaterOrEqual(t, after.TotalMs-before.TotalMs, int64(10), "total_ms")
	require.GreaterOrEqual(t, after.MaxMs, int64(10), "max_ms")
}

// testHealthReporter wraps a reporter that has its own recorder, logs JSON to a buffer,
// and reads its interval from a value the test can change.
type testHealthReporter struct {
	*HealthReporter
	logs          *threadsafebuffer.ThreadSafeBuffer
	interval      *atomic.Int64
	intervalReads *atomic.Int64
	done          chan error
}

func newTestHealthReporter(t *testing.T, interval time.Duration) *testHealthReporter {
	t.Helper()

	logs := &threadsafebuffer.ThreadSafeBuffer{}
	currentInterval := &atomic.Int64{}
	currentInterval.Store(int64(interval))
	intervalReads := &atomic.Int64{}

	k := typesmocks.NewKnapsack(t)
	k.On("Slogger").Return(slog.New(slog.NewJSONHandler(logs, nil)))
	k.On("RegisterChangeObserver", mock.Anything, keys.TableHealthReportInterval).Return()
	k.EXPECT().TableHealthReportInterval().RunAndReturn(func() time.Duration {
		return time.Duration(currentInterval.Load())
	}).Run(func() { intervalReads.Add(1) }).Maybe()
	k.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{OSName: "macOS", OSVersion: "26.7.1"}).Maybe()

	r := NewHealthReporter(k)
	r.recorder = newHealthRecorder()
	r.minInterval = time.Millisecond
	return &testHealthReporter{HealthReporter: r, logs: logs, interval: currentInterval, intervalReads: intervalReads}
}

func (tr *testHealthReporter) start(t *testing.T) {
	t.Helper()

	tr.done = make(chan error, 1)
	gowrapper.Go(t.Context(), multislogger.NewNopLogger(), func() {
		tr.done <- tr.Execute()
	})
	t.Cleanup(func() { tr.Interrupt(nil) })

	// Execute reads the interval once before it starts waiting; flag changes made by the
	// test after this point are changes to a running reporter.
	require.Eventually(t, func() bool { return tr.intervalReads.Load() > 0 }, 5*time.Second, time.Millisecond)
}

func (tr *testHealthReporter) setInterval(t *testing.T, d time.Duration) {
	t.Helper()

	tr.interval.Store(int64(d))
	tr.FlagsChanged(t.Context(), keys.TableHealthReportInterval)
}

// stop interrupts the reporter and waits for Execute to return.
func (tr *testHealthReporter) stop(t *testing.T) {
	t.Helper()

	tr.Interrupt(nil)
	tr.Interrupt(nil) // extra calls must not block
	select {
	case err := <-tr.done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Execute did not return after Interrupt")
	}
}

type loggedSummary struct {
	Msg           string        `json:"msg"`
	Level         string        `json:"level"`
	SchemaVersion int           `json:"schema_version"`
	RecordID      string        `json:"record_id"`
	WindowStart   time.Time     `json:"window_start"`
	WindowEnd     time.Time     `json:"window_end"`
	OSName        string        `json:"os_name"`
	OSVersion     string        `json:"os_version"`
	Arch          string        `json:"arch"`
	Tables        []TableCounts `json:"tables"`
}

// summaries returns every table_health_summary record logged so far.
func (tr *testHealthReporter) summaries(t *testing.T) []loggedSummary {
	t.Helper()

	var found []loggedSummary
	for line := range strings.SplitSeq(tr.logs.String(), "\n") {
		if line == "" {
			continue
		}
		var s loggedSummary
		require.NoError(t, json.Unmarshal([]byte(line), &s))
		if s.Msg == "table_health_summary" {
			found = append(found, s)
		}
	}
	return found
}

func TestHealthReporter_report(t *testing.T) {
	t.Parallel()

	tr := newTestHealthReporter(t, 6*time.Hour)
	tr.recorder.record("kolide_plist", outcomeEmpty, 40*time.Millisecond)
	tr.recorder.record("kolide_wifi_networks", outcomeError, 250*time.Millisecond)
	tr.recorder.record("kolide_wifi_networks", outcomeNoWorker, 0)

	tr.report(t.Context())

	// Check the raw keys, not just what unmarshals into the struct
	var raw map[string]any
	require.NoError(t, json.Unmarshal([]byte(tr.logs.String()), &raw))
	for _, key := range []string{"msg", "level", "schema_version", "record_id", "window_start", "window_end", "os_name", "os_version", "arch", "tables", "component"} {
		require.Contains(t, raw, key)
	}
	require.Equal(t, []any{
		map[string]any{"name": "kolide_plist", "query": float64(1), "error": float64(0), "empty": float64(1), "timeout": float64(0), "no_worker": float64(0), "total_ms": float64(40), "max_ms": float64(40)},
		map[string]any{"name": "kolide_wifi_networks", "query": float64(1), "error": float64(1), "empty": float64(0), "timeout": float64(0), "no_worker": float64(1), "total_ms": float64(250), "max_ms": float64(250)},
	}, raw["tables"])

	summaries := tr.summaries(t)
	require.Len(t, summaries, 1)
	logged := summaries[0]
	require.Equal(t, "INFO", logged.Level)
	require.Equal(t, healthSummarySchemaVersion, logged.SchemaVersion)
	require.NotEmpty(t, logged.RecordID)
	require.False(t, logged.WindowEnd.Before(logged.WindowStart))
	require.Equal(t, "macOS", logged.OSName)
	require.Equal(t, "26.7.1", logged.OSVersion)
	require.Equal(t, runtime.GOARCH, logged.Arch)

	// Nothing is logged when no table was queried
	tr.report(t.Context())
	require.Len(t, tr.summaries(t), 1)
}

func TestHealthReporter_flushesOnInterrupt(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name        string
		interval    time.Duration
		expectFlush bool
	}{
		{name: "enabled", interval: 6 * time.Hour, expectFlush: true},
		{name: "disabled", interval: 0, expectFlush: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tr := newTestHealthReporter(t, tt.interval)
			tr.recorder.record("kolide_plist", outcomeSuccess, 0)

			tr.start(t)
			tr.stop(t)

			require.Equal(t, tt.expectFlush, len(tr.summaries(t)) == 1)
		})
	}
}

func TestHealthReporter_FlagsChanged(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name            string
		initialInterval time.Duration
		newInterval     time.Duration
		// recordBefore is recorded before the flag changes; recordAfter is recorded repeatedly after
		recordBefore   string
		recordAfter    string
		expectedTables []string // tables expected across all summaries before Interrupt
	}{
		{
			name:            "shortened interval reports the open window promptly, once",
			initialInterval: 6 * time.Hour,
			newInterval:     20 * time.Millisecond,
			recordBefore:    "before_change",
			expectedTables:  []string{"before_change"},
		},
		{
			name:            "enabling from zero starts a fresh window",
			initialInterval: 0,
			newInterval:     20 * time.Millisecond,
			recordBefore:    "before_change",
			recordAfter:     "after_change",
			expectedTables:  []string{"after_change"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tr := newTestHealthReporter(t, tt.initialInterval)
			tr.recorder.record(tt.recordBefore, outcomeSuccess, 0)
			tr.start(t)
			tr.setInterval(t, tt.newInterval)

			expected := tt.expectedTables[len(tt.expectedTables)-1]
			require.Eventually(t, func() bool {
				if tt.recordAfter != "" {
					tr.recorder.record(tt.recordAfter, outcomeSuccess, 0)
				}
				for _, s := range tr.summaries(t) {
					for _, table := range s.Tables {
						if table.Name == expected {
							return true
						}
					}
				}
				return false
			}, 5*time.Second, 10*time.Millisecond, "no summary after the interval changed")
			tr.stop(t)

			// Each table shows up in exactly one record per window, and every window is distinct
			seen := make(map[string]int)
			var lastEnd time.Time
			for _, s := range tr.summaries(t) {
				require.False(t, s.WindowStart.Before(lastEnd), "windows must not overlap")
				lastEnd = s.WindowEnd
				for _, table := range s.Tables {
					seen[table.Name]++
				}
			}
			for name := range seen {
				require.Contains(t, tt.expectedTables, name)
			}
			if tt.recordAfter == "" {
				require.Equal(t, 1, seen[expected], "the open window must be reported exactly once")
			}
		})
	}
}

func TestHealthReporter_disablingStopsReporting(t *testing.T) {
	t.Parallel()

	tr := newTestHealthReporter(t, 6*time.Hour)
	tr.start(t)
	tr.setInterval(t, 0)

	// Once Execute has taken the change, nothing more is reported, not even on Interrupt
	require.Eventually(t, func() bool { return len(tr.intervalChanged) == 0 }, 5*time.Second, time.Millisecond)
	tr.recorder.record("kolide_plist", outcomeSuccess, 0)
	tr.stop(t)

	require.Empty(t, tr.summaries(t))
}

func TestJitter(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		d    time.Duration
	}{
		{name: "default interval", d: 6 * time.Hour},
		{name: "floor", d: minHealthReportInterval},
		{name: "max", d: 24 * time.Hour},
		{name: "tiny", d: 5 * time.Nanosecond},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lower := tt.d - tt.d/10
			upper := tt.d + tt.d/10
			for range 1000 {
				got := jitter(tt.d)
				require.GreaterOrEqual(t, got, lower)
				require.LessOrEqual(t, got, upper)
			}
		})
	}
}

func TestHealthReporter_currentInterval(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		flag     time.Duration
		expected time.Duration
	}{
		{name: "disabled", flag: 0, expected: 0},
		{name: "below floor is raised", flag: time.Minute, expected: minHealthReportInterval},
		{name: "at floor", flag: minHealthReportInterval, expected: minHealthReportInterval},
		{name: "default", flag: 6 * time.Hour, expected: 6 * time.Hour},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tr := newTestHealthReporter(t, tt.flag)
			tr.minInterval = minHealthReportInterval
			require.Equal(t, tt.expected, tr.currentInterval())
		})
	}
}
