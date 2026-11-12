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
type TableCounts struct {
	Name     string `json:"name"`
	Query    int64  `json:"query"`
	Error    int64  `json:"error"`
	Empty    int64  `json:"empty"`
	Timeout  int64  `json:"timeout"`
	NoWorker int64  `json:"no_worker"`
}

// healthRecorder accumulates per-table outcomes in memory between reports.
type healthRecorder struct {
	lock        sync.Mutex
	tables      map[string]*TableCounts
	windowStart time.Time
}

// tableHealth is process-wide, like the OTel counters it complements.
var tableHealth = newHealthRecorder()

func newHealthRecorder() *healthRecorder {
	return &healthRecorder{
		tables:      make(map[string]*TableCounts),
		windowStart: time.Now(),
	}
}

func (h *healthRecorder) record(tableName string, outcome queryOutcome) {
	h.lock.Lock()
	defer h.lock.Unlock()

	c, ok := h.tables[tableName]
	if !ok {
		c = &TableCounts{Name: tableName}
		h.tables[tableName] = c
	}

	switch outcome {
	case outcomeSuccess:
	case outcomeNoWorker:
		c.NoWorker++
		return
	case outcomeEmpty:
		c.Empty++
	case outcomeError:
		c.Error++
	case outcomeTimeout:
		c.Timeout++
	}
	c.Query++
}

// snapshotAndReset returns the counts since the last call, sorted by table name,
// along with the window they cover, and starts a new window.
func (h *healthRecorder) snapshotAndReset() ([]TableCounts, time.Time, time.Time) {
	h.lock.Lock()
	defer h.lock.Unlock()

	now := time.Now()
	windowStart := h.windowStart

	counts := make([]TableCounts, 0, len(h.tables))
	for _, c := range h.tables {
		counts = append(counts, *c)
	}
	slices.SortFunc(counts, func(a, b TableCounts) int {
		return strings.Compare(a.Name, b.Name)
	})

	h.tables = make(map[string]*TableCounts)
	h.windowStart = now

	return counts, windowStart, now
}

// windowStartedAt returns when the current window began.
func (h *healthRecorder) windowStartedAt() time.Time {
	h.lock.Lock()
	defer h.lock.Unlock()

	return h.windowStart
}
