package dedup

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
	"testing"
	"time"
)

// captureHandler removed; tests now capture emissions via nextCapture only.

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
	// Keep windows small to make test fast
	engine := New(
		WithDuplicateLogWindow(50*time.Millisecond),
		WithCleanupInterval(10*time.Millisecond),
		WithCacheExpiry(500*time.Millisecond),
	)
	engine.Start(context.Background())
	defer engine.Stop()

	mw := engine.Middleware
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

func TestDebugLevelIsDedupedLikeOthers(t *testing.T) {
	t.Parallel()

	next := &nextCapture{}
	engine := New(
		WithDuplicateLogWindow(50*time.Millisecond),
		WithCleanupInterval(10*time.Millisecond),
		WithCacheExpiry(500*time.Millisecond),
	)
	engine.Start(context.Background())
	defer engine.Stop()

	mw := engine.Middleware
	ctx := context.Background()

	// First debug log passes
	if err := mw(ctx, makeRecord(slog.LevelDebug, "debug msg"), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}
	// Immediate duplicate suppressed
	if err := mw(ctx, makeRecord(slog.LevelDebug, "debug msg"), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}
	if next.Len() != 1 {
		t.Fatalf("expected immediate duplicate to be suppressed, got %d", next.Len())
	}

	// After window, should re-log with count
	time.Sleep(60 * time.Millisecond)
	if err := mw(ctx, makeRecord(slog.LevelDebug, "debug msg"), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}
	if next.Len() != 2 {
		t.Fatalf("expected re-log after window, got %d", next.Len())
	}
	if val, ok := getAttrValue(next.Get(1), "duplicate_count"); !ok || val.Int64() != 3 {
		t.Fatalf("expected duplicate_count 3 on relogged debug record, got %v (ok=%v)", val, ok)
	}
}

func TestZeroWindowShortCircuitsDedup(t *testing.T) {
	t.Parallel()

	next := &nextCapture{}
	engine := New(
		WithDuplicateLogWindow(0), // disabled
	)
	engine.Start(context.Background())
	defer engine.Stop()

	mw := engine.Middleware
	ctx := context.Background()

	// First log
	if err := mw(ctx, makeRecord(slog.LevelInfo, "x", slog.String("k", "v")), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}
	// Duplicate immediately — should still pass because dedup is disabled
	if err := mw(ctx, makeRecord(slog.LevelInfo, "x", slog.String("k", "v")), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}

	if next.Len() != 2 {
		t.Fatalf("expected both records to pass when window disabled, got %d", next.Len())
	}

	// Ensure duplicate_count not automatically injected when disabled
	if _, ok := getAttrValue(next.Get(1), "duplicate_count"); ok {
		t.Fatalf("did not expect duplicate_count when window disabled")
	}
}

func TestCleanupEmitsSummaryRecordOnlyForDuplicates(t *testing.T) {
	t.Parallel()

	next := &nextCapture{}
	engine := New(
		WithDuplicateLogWindow(50*time.Millisecond),
		WithCleanupInterval(10*time.Millisecond),
		WithCacheExpiry(50*time.Millisecond),
	)
	engine.Start(context.Background())
	defer engine.Stop()

	mw := engine.Middleware
	ctx := context.Background()

	// Log a single record; should NOT emit a summary because count == 1
	if err := mw(ctx, makeRecord(slog.LevelInfo, "single msg", slog.String("a", "b")), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}

	// Wait for entry to expire and cleanup to run
	time.Sleep(150 * time.Millisecond)

	// Wait a little for any background cleanup emission to be handled
	initialPassCount := next.Len() // should be 1 from the first pass-through
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		if next.Len() > initialPassCount {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if next.Len() != initialPassCount {
		t.Fatalf("expected no summary emission for single record, got %d new records", next.Len()-initialPassCount)
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
	// We expect one new record (the summary) beyond the initial pass-through of the first dup
	expected := initialPassCount + 1 // from first pass-through in this section
	for time.Now().Before(deadline) {
		if next.Len() > expected { // expecting summary to make it expected+1
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if next.Len() <= expected {
		t.Fatalf("expected a summary record to be emitted for duplicates on cleanup")
	}

	summary := next.Get(next.Len() - 1)
	if summary.Message != "dup msg" {
		t.Fatalf("expected message %q, got %q", "dup msg", summary.Message)
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

func TestStopHaltsCleanupAndPreventsEmission(t *testing.T) {
	t.Parallel()

	next := &nextCapture{}
	engine := New(
		WithDuplicateLogWindow(200*time.Millisecond),
		WithCleanupInterval(30*time.Millisecond),
		WithCacheExpiry(80*time.Millisecond),
	)
	engine.Start(context.Background())

	mw := engine.Middleware
	ctx := context.Background()

	// Create a duplicate set which would normally lead to a summary emission after expiry
	if err := mw(ctx, makeRecord(slog.LevelInfo, "stop-test", slog.String("k", "v")), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}
	if err := mw(ctx, makeRecord(slog.LevelInfo, "stop-test", slog.String("k", "v")), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}

	// Stop the engine before the periodic cleanup can emit any summaries
	engine.Stop()

	// Wait beyond both cache expiry and cleanup interval; no summary should be emitted after Stop
	before := next.Len()
	time.Sleep(300 * time.Millisecond)

	if got := next.Len() - before; got != 0 {
		t.Fatalf("expected no summary emission after Stop, got %d new record(s)", got)
	}
}
