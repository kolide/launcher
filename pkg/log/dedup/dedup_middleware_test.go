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
	engine.Start(t.Context())
	defer engine.Stop()

	mw := engine.Middleware
	ctx := t.Context()

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
	engine.Start(t.Context())
	defer engine.Stop()

	mw := engine.Middleware
	ctx := t.Context()

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
	engine.Start(t.Context())
	defer engine.Stop()

	mw := engine.Middleware
	ctx := t.Context()

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
	engine.Start(t.Context())
	defer engine.Stop()

	mw := engine.Middleware
	ctx := t.Context()

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
	engine.Start(t.Context())

	mw := engine.Middleware
	ctx := t.Context()

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

func TestSetDuplicateLogWindow(t *testing.T) {
	t.Parallel()

	engine := New(WithDuplicateLogWindow(100 * time.Millisecond))

	// Test initial value
	if initial := engine.getDuplicateLogWindow(); initial != 100*time.Millisecond {
		t.Fatalf("expected initial window 100ms, got %v", initial)
	}

	// Test setting new value
	engine.SetDuplicateLogWindow(500 * time.Millisecond)
	if updated := engine.getDuplicateLogWindow(); updated != 500*time.Millisecond {
		t.Fatalf("expected updated window 500ms, got %v", updated)
	}

	// Test setting zero (disabling dedup)
	engine.SetDuplicateLogWindow(0)
	if disabled := engine.getDuplicateLogWindow(); disabled != 0 {
		t.Fatalf("expected disabled window 0, got %v", disabled)
	}
}

func TestSetDuplicateLogWindowAffectsMiddleware(t *testing.T) {
	t.Parallel()

	next := &nextCapture{}
	engine := New(WithDuplicateLogWindow(0)) // Start disabled
	engine.Start(t.Context())
	defer engine.Stop()

	mw := engine.Middleware
	ctx := t.Context()

	// With dedup disabled, duplicates should pass through
	if err := mw(ctx, makeRecord(slog.LevelInfo, "test", slog.String("k", "v")), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}
	if err := mw(ctx, makeRecord(slog.LevelInfo, "test", slog.String("k", "v")), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}

	if next.Len() != 2 {
		t.Fatalf("expected both records to pass when dedup disabled, got %d", next.Len())
	}

	// Enable dedup with a window
	engine.SetDuplicateLogWindow(100 * time.Millisecond)

	// Reset capture
	next.mu.Lock()
	next.records = nil
	next.mu.Unlock()

	// Now duplicates should be suppressed
	if err := mw(ctx, makeRecord(slog.LevelInfo, "test2", slog.String("k", "v")), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}
	if err := mw(ctx, makeRecord(slog.LevelInfo, "test2", slog.String("k", "v")), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}

	if next.Len() != 1 {
		t.Fatalf("expected duplicate to be suppressed when dedup enabled, got %d", next.Len())
	}
}

func TestSetDuplicateLogWindowConcurrentAccess(t *testing.T) {
	t.Parallel()

	engine := New(WithDuplicateLogWindow(50 * time.Millisecond))
	engine.Start(t.Context())
	defer engine.Stop()

	next := &nextCapture{}
	mw := engine.Middleware
	ctx := t.Context()

	// Start multiple goroutines that concurrently update the window and process logs
	var wg sync.WaitGroup
	done := make(chan struct{})

	// Goroutine 1: Continuously update the window
	wg.Go(func() {
		windows := []time.Duration{0, 50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond}
		i := 0
		for {
			select {
			case <-done:
				return
			default:
				engine.SetDuplicateLogWindow(windows[i%len(windows)])
				i++
				time.Sleep(10 * time.Millisecond)
			}
		}
	})

	// Goroutine 2: Continuously process logs
	wg.Go(func() {
		for i := range 100 {
			select {
			case <-done:
				return
			default:
				record := makeRecord(slog.LevelInfo, "concurrent test", slog.Int("i", i))
				if err := mw(ctx, record, next.next); err != nil {
					t.Errorf("middleware err: %v", err)
				}
				time.Sleep(5 * time.Millisecond)
			}
		}
	})

	// Goroutine 3: Continuously read the window
	wg.Go(func() {
		for range 100 {
			select {
			case <-done:
				return
			default:
				window := engine.getDuplicateLogWindow()
				_ = window // Just read it, don't need to verify specific value due to concurrent updates
				time.Sleep(3 * time.Millisecond)
			}
		}
	})

	// Let the test run for a short duration
	time.Sleep(500 * time.Millisecond)
	close(done)
	wg.Wait()

	// Test passes if no race conditions occurred (detected by go test -race)
}

// TestEntryResetsAfterWindowElapses verifies that after the dedup window elapses
// and a log is emitted with duplicate metadata, the entry is reset so subsequent
// logs start a fresh deduplication window. This prevents the bug where duplicate_count
// would keep incrementing forever and first_seen would never update.
func TestEntryResetsAfterWindowElapses(t *testing.T) {
	t.Parallel()

	next := &nextCapture{}
	engine := New(
		WithDuplicateLogWindow(50*time.Millisecond),
		WithCleanupInterval(1*time.Second), // Long interval to avoid cleanup interference
		WithCacheExpiry(10*time.Second),    // Long expiry to avoid cleanup interference
	)
	engine.Start(t.Context())
	defer engine.Stop()

	mw := engine.Middleware
	ctx := t.Context()

	// First log: passes through (count=1, no duplicate metadata)
	if err := mw(ctx, makeRecord(slog.LevelInfo, "reset-test", slog.String("k", "v")), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}
	if next.Len() != 1 {
		t.Fatalf("expected first record to pass, got %d", next.Len())
	}
	// First record should NOT have duplicate_count
	if _, ok := getAttrValue(next.Get(0), "duplicate_count"); ok {
		t.Fatalf("first record should not have duplicate_count")
	}

	// Second log within window: suppressed
	if err := mw(ctx, makeRecord(slog.LevelInfo, "reset-test", slog.String("k", "v")), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}
	if next.Len() != 1 {
		t.Fatalf("expected duplicate to be suppressed, got %d", next.Len())
	}

	// Wait for window to elapse
	time.Sleep(60 * time.Millisecond)

	// Third log after window: emitted with duplicate metadata, then entry resets
	if err := mw(ctx, makeRecord(slog.LevelInfo, "reset-test", slog.String("k", "v")), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}
	if next.Len() != 2 {
		t.Fatalf("expected re-log after window, got %d", next.Len())
	}

	// Verify duplicate metadata on emitted record
	r := next.Get(1)
	if val, ok := getAttrValue(r, "duplicate_count"); !ok || val.Int64() != 3 {
		t.Fatalf("expected duplicate_count 3, got %v", val)
	}
	if _, ok := getAttrValue(r, "first_seen"); !ok {
		t.Fatalf("expected first_seen attribute")
	}
	if _, ok := getAttrValue(r, "last_seen"); !ok {
		t.Fatalf("expected last_seen attribute")
	}
	// Middleware emissions should NOT have original_msg (only cleanup does)
	if _, ok := getAttrValue(r, "original_msg"); ok {
		t.Fatalf("middleware emission should not have original_msg attribute")
	}

	// Fourth log immediately after reset: suppressed (new window started, count=2)
	if err := mw(ctx, makeRecord(slog.LevelInfo, "reset-test", slog.String("k", "v")), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}
	if next.Len() != 2 {
		t.Fatalf("expected duplicate after reset to be suppressed, got %d", next.Len())
	}

	// Wait for new window to elapse
	time.Sleep(60 * time.Millisecond)

	// Fifth log: emitted with NEW duplicate metadata (reset worked!)
	if err := mw(ctx, makeRecord(slog.LevelInfo, "reset-test", slog.String("k", "v")), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}
	if next.Len() != 3 {
		t.Fatalf("expected second re-log after reset, got %d", next.Len())
	}

	// Verify the NEW duplicate_count is 3 (not 5+), proving reset worked
	r2 := next.Get(2)
	if val, ok := getAttrValue(r2, "duplicate_count"); !ok {
		t.Fatalf("expected duplicate_count on second emission")
	} else if val.Int64() != 3 {
		t.Fatalf("expected duplicate_count 3 after reset (not accumulated), got %d", val.Int64())
	}
}

// TestMultipleWindowCycles verifies that the dedup engine correctly handles
// multiple window cycles, resetting the entry each time and maintaining
// accurate counts per cycle.
func TestMultipleWindowCycles(t *testing.T) {
	t.Parallel()

	next := &nextCapture{}
	engine := New(
		WithDuplicateLogWindow(30*time.Millisecond),
		WithCleanupInterval(1*time.Second),
		WithCacheExpiry(10*time.Second),
	)
	engine.Start(t.Context())
	defer engine.Stop()

	mw := engine.Middleware
	ctx := t.Context()

	// Run through 3 complete window cycles
	for cycle := range 3 {
		expectedRecords := cycle + 1 // One emission per cycle

		// First log of cycle (or first log ever): passes through
		if cycle == 0 {
			if err := mw(ctx, makeRecord(slog.LevelInfo, "cycle-test"), next.next); err != nil {
				t.Fatalf("cycle %d: middleware err: %v", cycle, err)
			}
			if next.Len() != 1 {
				t.Fatalf("cycle %d: expected first record to pass, got %d", cycle, next.Len())
			}
		}

		// Add more duplicates within window
		for range 2 {
			if err := mw(ctx, makeRecord(slog.LevelInfo, "cycle-test"), next.next); err != nil {
				t.Fatalf("cycle %d: middleware err: %v", cycle, err)
			}
		}

		// Wait for window to elapse
		time.Sleep(40 * time.Millisecond)

		// Trigger emission with next log
		if err := mw(ctx, makeRecord(slog.LevelInfo, "cycle-test"), next.next); err != nil {
			t.Fatalf("cycle %d: middleware err: %v", cycle, err)
		}

		// Verify we got the expected emission
		if next.Len() != expectedRecords+1 {
			t.Fatalf("cycle %d: expected %d records, got %d", cycle, expectedRecords+1, next.Len())
		}

		// Verify duplicate_count is reasonable (not accumulating across cycles)
		lastRecord := next.Get(next.Len() - 1)
		if val, ok := getAttrValue(lastRecord, "duplicate_count"); ok {
			// Count should be 3-4 per cycle, not growing unbounded
			if val.Int64() > 5 {
				t.Fatalf("cycle %d: duplicate_count %d is too high, suggests reset not working", cycle, val.Int64())
			}
		}
	}
}

// TestCleanupEmissionsHaveOriginalMsg verifies that logs emitted via the
// background cleanup path include the original_msg attribute, distinguishing
// them from logs emitted via the middleware window-elapsed path.
func TestCleanupEmissionsHaveOriginalMsg(t *testing.T) {
	t.Parallel()

	next := &nextCapture{}
	engine := New(
		WithDuplicateLogWindow(100*time.Millisecond), // Long window so cleanup triggers first
		WithCleanupInterval(20*time.Millisecond),
		WithCacheExpiry(50*time.Millisecond), // Short expiry for quick cleanup
	)
	engine.Start(t.Context())
	defer engine.Stop()

	mw := engine.Middleware
	ctx := t.Context()

	// Create duplicates that will be cleaned up (not window-elapsed)
	if err := mw(ctx, makeRecord(slog.LevelInfo, "cleanup-test", slog.String("key", "value")), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}
	if err := mw(ctx, makeRecord(slog.LevelInfo, "cleanup-test", slog.String("key", "value")), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}

	initialCount := next.Len() // Should be 1 (first pass-through)

	// Wait for cache expiry + cleanup to run (but NOT window elapsed)
	time.Sleep(150 * time.Millisecond)

	// Cleanup should have emitted a summary
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) && next.Len() <= initialCount {
		time.Sleep(10 * time.Millisecond)
	}

	if next.Len() <= initialCount {
		t.Fatalf("expected cleanup to emit summary record")
	}

	// Verify cleanup emission has original_msg attribute
	cleanupRecord := next.Get(next.Len() - 1)
	if val, ok := getAttrValue(cleanupRecord, "original_msg"); !ok {
		t.Fatalf("cleanup emission should have original_msg attribute")
	} else if val.String() != "cleanup-test" {
		t.Fatalf("expected original_msg 'cleanup-test', got %q", val.String())
	}

	// Cleanup should also have the other standard attributes
	if _, ok := getAttrValue(cleanupRecord, "duplicate_count"); !ok {
		t.Fatalf("cleanup emission should have duplicate_count")
	}
	if _, ok := getAttrValue(cleanupRecord, "first_seen"); !ok {
		t.Fatalf("cleanup emission should have first_seen")
	}
	if _, ok := getAttrValue(cleanupRecord, "last_seen"); !ok {
		t.Fatalf("cleanup emission should have last_seen")
	}
}

// TestMiddlewareEmissionsLackOriginalMsg verifies that logs emitted via the
// middleware window-elapsed path do NOT include the original_msg attribute,
// distinguishing them from cleanup emissions.
func TestMiddlewareEmissionsLackOriginalMsg(t *testing.T) {
	t.Parallel()

	next := &nextCapture{}
	engine := New(
		WithDuplicateLogWindow(30*time.Millisecond),
		WithCleanupInterval(1*time.Second), // Long interval to prevent cleanup
		WithCacheExpiry(10*time.Second),    // Long expiry to prevent cleanup
	)
	engine.Start(t.Context())
	defer engine.Stop()

	mw := engine.Middleware
	ctx := t.Context()

	// First log passes through
	if err := mw(ctx, makeRecord(slog.LevelInfo, "mw-test"), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}

	// Add duplicate
	if err := mw(ctx, makeRecord(slog.LevelInfo, "mw-test"), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}

	// Wait for window to elapse
	time.Sleep(40 * time.Millisecond)

	// Trigger middleware emission
	if err := mw(ctx, makeRecord(slog.LevelInfo, "mw-test"), next.next); err != nil {
		t.Fatalf("middleware err: %v", err)
	}

	if next.Len() != 2 {
		t.Fatalf("expected 2 records, got %d", next.Len())
	}

	// Verify middleware emission does NOT have original_msg
	mwRecord := next.Get(1)
	if _, ok := getAttrValue(mwRecord, "original_msg"); ok {
		t.Fatalf("middleware emission should NOT have original_msg attribute")
	}

	// But it should have the other attributes
	if _, ok := getAttrValue(mwRecord, "duplicate_count"); !ok {
		t.Fatalf("middleware emission should have duplicate_count")
	}
	if _, ok := getAttrValue(mwRecord, "first_seen"); !ok {
		t.Fatalf("middleware emission should have first_seen")
	}
	if _, ok := getAttrValue(mwRecord, "last_seen"); !ok {
		t.Fatalf("middleware emission should have last_seen")
	}
}
