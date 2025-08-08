package dedup

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
	"testing"
	"time"
)

// captureHandler is a slog.Handler implementation that captures handled records.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(name string) slog.Handler       { return h }

func (h *captureHandler) Len() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.records)
}

func (h *captureHandler) Get(i int) slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.records[i]
}

// nextCapture captures records that pass through the middleware to the next handler.
type nextCapture struct {
	mu      sync.Mutex
	records []slog.Record
}

func (n *nextCapture) next(_ context.Context, r slog.Record) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.records = append(n.records, r.Clone())
	return nil
}

func (n *nextCapture) Len() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.records)
}

func (n *nextCapture) Get(i int) slog.Record {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.records[i]
}

func makeRecord(level slog.Level, msg string, attrs ...slog.Attr) slog.Record {
	// capture the caller PC to simulate real logger behavior
	pc, _, _, _ := runtime.Caller(1)
	r := slog.NewRecord(time.Now(), level, msg, pc)
	r.AddAttrs(attrs...)
	return r
}

func getAttrValue(r slog.Record, key string) (slog.Value, bool) {
	var v slog.Value
	found := false
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			v = a.Value
			found = true
			return false
		}
		return true
	})
	return v, found
}

func TestDedupSuppressesAndRelogsWithCount(t *testing.T) {
	t.Parallel()

	next := &nextCapture{}
	sink := &captureHandler{}
	// Keep windows small to make test fast
	engine := New(slog.New(sink),
		WithDuplicateLogWindow(50*time.Millisecond),
		WithCleanupInterval(10*time.Millisecond),
		WithCacheExpiry(500*time.Millisecond),
	)
	defer engine.Stop()

	mw := Middleware(engine)
	ctx := context.Background()

	// First log: allowed through
	if err := mw(ctx, makeRecord(slog.LevelInfo, "hello", slog.String("k", "v")), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}
	if next.Len() != 1 {
		t.Fatalf("expected first record to pass, got %d", next.Len())
	}

	// Immediate duplicate: suppressed
	if err := mw(ctx, makeRecord(slog.LevelInfo, "hello", slog.String("k", "v")), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}
	if next.Len() != 1 {
		t.Fatalf("expected duplicate to be suppressed within window, got %d", next.Len())
	}

	// After window: should re-log with duplicate_count set to 3
	time.Sleep(60 * time.Millisecond)
	if err := mw(ctx, makeRecord(slog.LevelInfo, "hello", slog.String("k", "v")), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}
	if next.Len() != 2 {
		t.Fatalf("expected re-log after window, got %d", next.Len())
	}

	// Verify duplicate_count attribute on the second passed record
	r := next.Get(1)
	if val, ok := getAttrValue(r, "duplicate_count"); !ok {
		t.Fatalf("expected duplicate_count attribute on relogged record")
	} else {
		if val.Kind() != slog.KindInt64 {
			t.Fatalf("expected duplicate_count to be int64 kind, got %v", val.Kind())
		}
		if val.Int64() != 3 {
			t.Fatalf("expected duplicate_count 3, got %d", val.Int64())
		}
	}
}

func TestDebugLevelBypassesDedup(t *testing.T) {
	t.Parallel()

	next := &nextCapture{}
	sink := &captureHandler{}
	engine := New(slog.New(sink),
		WithDuplicateLogWindow(100*time.Millisecond),
		WithCleanupInterval(10*time.Millisecond),
		WithCacheExpiry(500*time.Millisecond),
	)
	defer engine.Stop()

	mw := Middleware(engine)
	ctx := context.Background()

	if err := mw(ctx, makeRecord(slog.LevelDebug, "debug msg"), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}
	if err := mw(ctx, makeRecord(slog.LevelDebug, "debug msg"), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}

	if next.Len() != 2 {
		t.Fatalf("expected all debug logs to pass, got %d", next.Len())
	}
}

func TestEmittedAttrSkipsDedup(t *testing.T) {
	t.Parallel()

	next := &nextCapture{}
	sink := &captureHandler{}
	engine := New(slog.New(sink),
		WithDuplicateLogWindow(200*time.Millisecond),
		WithCleanupInterval(10*time.Millisecond),
		WithCacheExpiry(500*time.Millisecond),
	)
	defer engine.Stop()

	mw := Middleware(engine)
	ctx := context.Background()

	// First record marked as emitted should bypass dedup
	r := makeRecord(slog.LevelInfo, "info msg", slog.Bool(EmittedAttrKey, true))
	if err := mw(ctx, r, next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}

	// Immediately log same message without emitted attr: should be treated as first occurrence
	if err := mw(ctx, makeRecord(slog.LevelInfo, "info msg"), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}

	// Immediate duplicate should be suppressed
	if err := mw(ctx, makeRecord(slog.LevelInfo, "info msg"), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}

	if next.Len() != 2 {
		t.Fatalf("expected two passed records (emitted + first real), got %d", next.Len())
	}
}

func TestCleanupEmitsSummaryRecordOnlyForDuplicates(t *testing.T) {
	t.Parallel()

	next := &nextCapture{}
	sink := &captureHandler{}
	engine := New(slog.New(sink),
		WithDuplicateLogWindow(50*time.Millisecond),
		WithCleanupInterval(10*time.Millisecond),
		WithCacheExpiry(50*time.Millisecond),
	)
	defer engine.Stop()

	mw := Middleware(engine)
	ctx := context.Background()

	// Log a single record; should NOT emit a summary because count == 1
	if err := mw(ctx, makeRecord(slog.LevelInfo, "single msg", slog.String("a", "b")), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}

	// Wait for entry to expire and cleanup to run
	time.Sleep(150 * time.Millisecond)

	// Wait a little for any background cleanup emission to be handled
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		if sink.Len() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if sink.Len() != 0 {
		t.Fatalf("expected no summary emission for single record, got %d", sink.Len())
	}

	// Now log a duplicate set; should emit a summary (count > 1)
	if err := mw(ctx, makeRecord(slog.LevelInfo, "dup msg", slog.String("k", "v")), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}
	if err := mw(ctx, makeRecord(slog.LevelInfo, "dup msg", slog.String("k", "v")), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}

	time.Sleep(150 * time.Millisecond)
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if sink.Len() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if sink.Len() == 0 {
		t.Fatalf("expected a summary record to be emitted for duplicates on cleanup")
	}

	summary := sink.Get(0)
	if summary.Message != duplicateSummaryMsg {
		t.Fatalf("expected message %q, got %q", duplicateSummaryMsg, summary.Message)
	}
	if v, ok := getAttrValue(summary, EmittedAttrKey); !ok || v.Kind() != slog.KindBool || !v.Bool() {
		t.Fatalf("expected %s to be true on summary record", EmittedAttrKey)
	}
	if v, ok := getAttrValue(summary, "duplicate_count"); !ok || v.Kind() != slog.KindInt64 || v.Int64() < 2 {
		t.Fatalf("expected duplicate_count >= 2 on summary record, got %v", v)
	}
	if v, ok := getAttrValue(summary, "original_msg"); !ok || v.Kind() != slog.KindString || v.String() != "dup msg" {
		t.Fatalf("expected original_msg 'dup msg', got %q", v.String())
	}
	if summary.PC == 0 {
		t.Fatalf("expected summary record to preserve original PC (call site)")
	}
}
