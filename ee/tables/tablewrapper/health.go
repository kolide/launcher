package tablewrapper

import (
	"slices"
	"strings"
	"sync"
	"time"
)

// queryOutcome is the result of one call to a wrapped table's generate function.
type queryOutcome int

const (
	outcomeSuccess queryOutcome = iota
	outcomeEmpty
	outcomeError
	outcomeTimeout
	outcomeNoWorker
)

// TableCounts holds per-table query outcomes for one reporting window.
// Every completed call counts as a query; timeouts are queries but not errors.
// TotalMs and MaxMs cover the calls counted in Query: the time they took in total, and the
// longest single call. A call that times out counts for as long as we waited for it.
// Zero outcome counts are left out of the JSON to keep the record under the log shipper's
// 8 KB limit per log line; a missing count means 0.
type TableCounts struct {
	Name     string `json:"name"`
	Query    int64  `json:"query"`
	Error    int64  `json:"error,omitempty"`
	Empty    int64  `json:"empty,omitempty"`
	Timeout  int64  `json:"timeout,omitempty"`
	NoWorker int64  `json:"no_worker,omitempty"`
	TotalMs  int64  `json:"total_ms"`
	MaxMs    int64  `json:"max_ms"`
}

// tableStats is the running state for one table. Durations are kept at full resolution and
// only rounded to milliseconds when read, so many sub-millisecond calls still add up.
type tableStats struct {
	counts TableCounts
	total  time.Duration
	max    time.Duration
}

// read returns the counts with the durations filled in.
func (s *tableStats) read() TableCounts {
	c := s.counts
	c.TotalMs = s.total.Round(time.Millisecond).Milliseconds()
	c.MaxMs = s.max.Round(time.Millisecond).Milliseconds()
	return c
}

// healthRecorder accumulates per-table outcomes in memory between reports.
type healthRecorder struct {
	lock        sync.Mutex
	tables      map[string]*tableStats
	windowStart time.Time
}

// tableHealth is process-wide, like the OTel counters it complements.
var tableHealth = newHealthRecorder()

func newHealthRecorder() *healthRecorder {
	return &healthRecorder{
		tables:      make(map[string]*tableStats),
		windowStart: time.Now(),
	}
}

// record counts one call to a table. elapsed is how long the call took; it is ignored for
// outcomeNoWorker, where the table was never called.
func (h *healthRecorder) record(tableName string, outcome queryOutcome, elapsed time.Duration) {
	h.lock.Lock()
	defer h.lock.Unlock()

	s, ok := h.tables[tableName]
	if !ok {
		s = &tableStats{counts: TableCounts{Name: tableName}}
		h.tables[tableName] = s
	}

	switch outcome {
	case outcomeSuccess:
	case outcomeNoWorker:
		s.counts.NoWorker++
		return
	case outcomeEmpty:
		s.counts.Empty++
	case outcomeError:
		s.counts.Error++
	case outcomeTimeout:
		s.counts.Timeout++
	}
	s.counts.Query++
	s.total += elapsed
	s.max = max(s.max, elapsed)
}

// snapshotAndReset returns the counts since the last call, sorted by table name,
// along with the window they cover, and starts a new window.
func (h *healthRecorder) snapshotAndReset() ([]TableCounts, time.Time, time.Time) {
	h.lock.Lock()
	defer h.lock.Unlock()

	now := time.Now()
	windowStart := h.windowStart

	counts := make([]TableCounts, 0, len(h.tables))
	for _, s := range h.tables {
		counts = append(counts, s.read())
	}
	slices.SortFunc(counts, func(a, b TableCounts) int {
		return strings.Compare(a.Name, b.Name)
	})

	h.tables = make(map[string]*tableStats)
	h.windowStart = now

	return counts, windowStart, now
}
