package dedup

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"runtime"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// captureHandler implements slog.Handler and records every record it receives.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }

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

func (h *captureHandler) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = nil
}

func makeRecord(level slog.Level, msg string, attrs ...slog.Attr) slog.Record {
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

// newTestHandler creates a dedup handler wrapping the given capture handler.
func newTestHandler(engine *Engine, capture *captureHandler) slog.Handler {
	return engine.NewMiddleware()(capture)
}

func TestDedupSuppressesAndRelogsWithCount(t *testing.T) {
	t.Parallel()

	capture := &captureHandler{}
	engine := New(
		WithDuplicateLogWindow(50*time.Millisecond),
		WithCleanupInterval(10*time.Millisecond),
		WithCacheExpiry(500*time.Millisecond),
	)
	engine.Start(t.Context())
	defer engine.Stop()

	handler := newTestHandler(engine, capture)
	ctx := t.Context()

	// First log: allowed through
	if err := handler.Handle(ctx, makeRecord(slog.LevelInfo, "hello", slog.String("k", "v"))); err != nil {
		t.Fatalf("handle err: %v", err)
	}
	if capture.Len() != 1 {
		t.Fatalf("expected first record to pass, got %d", capture.Len())
	}

	// Immediate duplicate: suppressed
	if err := handler.Handle(ctx, makeRecord(slog.LevelInfo, "hello", slog.String("k", "v"))); err != nil {
		t.Fatalf("handle err: %v", err)
	}
	if capture.Len() != 1 {
		t.Fatalf("expected duplicate to be suppressed within window, got %d", capture.Len())
	}

	// After window: should re-log with duplicate_count set to 3
	time.Sleep(60 * time.Millisecond)
	if err := handler.Handle(ctx, makeRecord(slog.LevelInfo, "hello", slog.String("k", "v"))); err != nil {
		t.Fatalf("handle err: %v", err)
	}
	if capture.Len() != 2 {
		t.Fatalf("expected re-log after window, got %d", capture.Len())
	}

	r := capture.Get(1)
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

	capture := &captureHandler{}
	engine := New(
		WithDuplicateLogWindow(50*time.Millisecond),
		WithCleanupInterval(10*time.Millisecond),
		WithCacheExpiry(500*time.Millisecond),
	)
	engine.Start(t.Context())
	defer engine.Stop()

	handler := newTestHandler(engine, capture)
	ctx := t.Context()

	if err := handler.Handle(ctx, makeRecord(slog.LevelDebug, "debug msg")); err != nil {
		t.Fatalf("handle err: %v", err)
	}
	if err := handler.Handle(ctx, makeRecord(slog.LevelDebug, "debug msg")); err != nil {
		t.Fatalf("handle err: %v", err)
	}
	if capture.Len() != 1 {
		t.Fatalf("expected immediate duplicate to be suppressed, got %d", capture.Len())
	}

	time.Sleep(60 * time.Millisecond)
	if err := handler.Handle(ctx, makeRecord(slog.LevelDebug, "debug msg")); err != nil {
		t.Fatalf("handle err: %v", err)
	}
	if capture.Len() != 2 {
		t.Fatalf("expected re-log after window, got %d", capture.Len())
	}
	if val, ok := getAttrValue(capture.Get(1), "duplicate_count"); !ok || val.Int64() != 3 {
		t.Fatalf("expected duplicate_count 3 on relogged debug record, got %v (ok=%v)", val, ok)
	}
}

func TestZeroWindowShortCircuitsDedup(t *testing.T) {
	t.Parallel()

	capture := &captureHandler{}
	engine := New(
		WithDuplicateLogWindow(0), // disabled
	)
	engine.Start(t.Context())
	defer engine.Stop()

	handler := newTestHandler(engine, capture)
	ctx := t.Context()

	if err := handler.Handle(ctx, makeRecord(slog.LevelInfo, "x", slog.String("k", "v"))); err != nil {
		t.Fatalf("handle err: %v", err)
	}
	if err := handler.Handle(ctx, makeRecord(slog.LevelInfo, "x", slog.String("k", "v"))); err != nil {
		t.Fatalf("handle err: %v", err)
	}

	if capture.Len() != 2 {
		t.Fatalf("expected both records to pass when window disabled, got %d", capture.Len())
	}

	if _, ok := getAttrValue(capture.Get(1), "duplicate_count"); ok {
		t.Fatalf("did not expect duplicate_count when window disabled")
	}
}

func TestCleanupEmitsSummaryRecordOnlyForDuplicates(t *testing.T) {
	t.Parallel()

	capture := &captureHandler{}
	engine := New(
		WithDuplicateLogWindow(50*time.Millisecond),
		WithCleanupInterval(10*time.Millisecond),
		WithCacheExpiry(50*time.Millisecond),
	)
	engine.Start(t.Context())
	defer engine.Stop()

	handler := newTestHandler(engine, capture)
	ctx := t.Context()

	// Log a single record; should NOT emit a summary because count == 1
	if err := handler.Handle(ctx, makeRecord(slog.LevelInfo, "single msg", slog.String("a", "b"))); err != nil {
		t.Fatalf("handle err: %v", err)
	}

	time.Sleep(150 * time.Millisecond)

	initialPassCount := capture.Len()
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		if capture.Len() > initialPassCount {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if capture.Len() != initialPassCount {
		t.Fatalf("expected no summary emission for single record, got %d new records", capture.Len()-initialPassCount)
	}

	// Now log a duplicate set; should emit a summary (count > 1)
	if err := handler.Handle(ctx, makeRecord(slog.LevelInfo, "dup msg", slog.String("k", "v"))); err != nil {
		t.Fatalf("handle err: %v", err)
	}
	if err := handler.Handle(ctx, makeRecord(slog.LevelInfo, "dup msg", slog.String("k", "v"))); err != nil {
		t.Fatalf("handle err: %v", err)
	}

	time.Sleep(150 * time.Millisecond)
	deadline = time.Now().Add(500 * time.Millisecond)
	expected := initialPassCount + 1
	for time.Now().Before(deadline) {
		if capture.Len() > expected {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if capture.Len() <= expected {
		t.Fatalf("expected a summary record to be emitted for duplicates on cleanup")
	}

	summary := capture.Get(capture.Len() - 1)
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

	capture := &captureHandler{}
	engine := New(
		WithDuplicateLogWindow(200*time.Millisecond),
		WithCleanupInterval(30*time.Millisecond),
		WithCacheExpiry(80*time.Millisecond),
	)
	engine.Start(t.Context())

	handler := newTestHandler(engine, capture)
	ctx := t.Context()

	if err := handler.Handle(ctx, makeRecord(slog.LevelInfo, "stop-test", slog.String("k", "v"))); err != nil {
		t.Fatalf("handle err: %v", err)
	}
	if err := handler.Handle(ctx, makeRecord(slog.LevelInfo, "stop-test", slog.String("k", "v"))); err != nil {
		t.Fatalf("handle err: %v", err)
	}

	engine.Stop()

	before := capture.Len()
	time.Sleep(300 * time.Millisecond)

	if got := capture.Len() - before; got != 0 {
		t.Fatalf("expected no summary emission after Stop, got %d new record(s)", got)
	}
}

func TestSetDuplicateLogWindow(t *testing.T) {
	t.Parallel()

	engine := New(WithDuplicateLogWindow(100 * time.Millisecond))

	if initial := engine.getDuplicateLogWindow(); initial != 100*time.Millisecond {
		t.Fatalf("expected initial window 100ms, got %v", initial)
	}

	engine.SetDuplicateLogWindow(500 * time.Millisecond)
	if updated := engine.getDuplicateLogWindow(); updated != 500*time.Millisecond {
		t.Fatalf("expected updated window 500ms, got %v", updated)
	}

	engine.SetDuplicateLogWindow(0)
	if disabled := engine.getDuplicateLogWindow(); disabled != 0 {
		t.Fatalf("expected disabled window 0, got %v", disabled)
	}
}

func TestSetDuplicateLogWindowAffectsHandler(t *testing.T) {
	t.Parallel()

	capture := &captureHandler{}
	engine := New(WithDuplicateLogWindow(0)) // Start disabled
	engine.Start(t.Context())
	defer engine.Stop()

	handler := newTestHandler(engine, capture)
	ctx := t.Context()

	// With dedup disabled, duplicates should pass through
	if err := handler.Handle(ctx, makeRecord(slog.LevelInfo, "test", slog.String("k", "v"))); err != nil {
		t.Fatalf("handle err: %v", err)
	}
	if err := handler.Handle(ctx, makeRecord(slog.LevelInfo, "test", slog.String("k", "v"))); err != nil {
		t.Fatalf("handle err: %v", err)
	}

	if capture.Len() != 2 {
		t.Fatalf("expected both records to pass when dedup disabled, got %d", capture.Len())
	}

	// Enable dedup with a window
	engine.SetDuplicateLogWindow(100 * time.Millisecond)
	capture.Reset()

	// Now duplicates should be suppressed
	if err := handler.Handle(ctx, makeRecord(slog.LevelInfo, "test2", slog.String("k", "v"))); err != nil {
		t.Fatalf("handle err: %v", err)
	}
	if err := handler.Handle(ctx, makeRecord(slog.LevelInfo, "test2", slog.String("k", "v"))); err != nil {
		t.Fatalf("handle err: %v", err)
	}

	if capture.Len() != 1 {
		t.Fatalf("expected duplicate to be suppressed when dedup enabled, got %d", capture.Len())
	}
}

func TestSetDuplicateLogWindowConcurrentAccess(t *testing.T) {
	t.Parallel()

	capture := &captureHandler{}
	engine := New(WithDuplicateLogWindow(50 * time.Millisecond))
	engine.Start(t.Context())
	defer engine.Stop()

	handler := newTestHandler(engine, capture)
	ctx := t.Context()

	var wg sync.WaitGroup
	done := make(chan struct{})

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

	wg.Go(func() {
		for i := range 100 {
			select {
			case <-done:
				return
			default:
				record := makeRecord(slog.LevelInfo, "concurrent test", slog.Int("i", i))
				if err := handler.Handle(ctx, record); err != nil {
					t.Errorf("handle err: %v", err)
				}
				time.Sleep(5 * time.Millisecond)
			}
		}
	})

	wg.Go(func() {
		for range 100 {
			select {
			case <-done:
				return
			default:
				_ = engine.getDuplicateLogWindow()
				time.Sleep(3 * time.Millisecond)
			}
		}
	})

	time.Sleep(500 * time.Millisecond)
	close(done)
	wg.Wait()
}

// TestEntryResetsAfterWindowElapses verifies that after the dedup window elapses
// and a log is emitted with duplicate metadata, the entry is reset so subsequent
// logs start a fresh deduplication window.
func TestEntryResetsAfterWindowElapses(t *testing.T) {
	t.Parallel()

	capture := &captureHandler{}
	engine := New(
		WithDuplicateLogWindow(50*time.Millisecond),
		WithCleanupInterval(1*time.Second),
		WithCacheExpiry(10*time.Second),
	)
	engine.Start(t.Context())
	defer engine.Stop()

	handler := newTestHandler(engine, capture)
	ctx := t.Context()

	// First log: passes through (count=1, no duplicate metadata)
	if err := handler.Handle(ctx, makeRecord(slog.LevelInfo, "reset-test", slog.String("k", "v"))); err != nil {
		t.Fatalf("handle err: %v", err)
	}
	if capture.Len() != 1 {
		t.Fatalf("expected first record to pass, got %d", capture.Len())
	}
	if _, ok := getAttrValue(capture.Get(0), "duplicate_count"); ok {
		t.Fatalf("first record should not have duplicate_count")
	}

	// Second log within window: suppressed
	if err := handler.Handle(ctx, makeRecord(slog.LevelInfo, "reset-test", slog.String("k", "v"))); err != nil {
		t.Fatalf("handle err: %v", err)
	}
	if capture.Len() != 1 {
		t.Fatalf("expected duplicate to be suppressed, got %d", capture.Len())
	}

	time.Sleep(60 * time.Millisecond)

	// Third log after window: emitted with duplicate metadata, then entry resets
	if err := handler.Handle(ctx, makeRecord(slog.LevelInfo, "reset-test", slog.String("k", "v"))); err != nil {
		t.Fatalf("handle err: %v", err)
	}
	if capture.Len() != 2 {
		t.Fatalf("expected re-log after window, got %d", capture.Len())
	}

	r := capture.Get(1)
	if val, ok := getAttrValue(r, "duplicate_count"); !ok || val.Int64() != 3 {
		t.Fatalf("expected duplicate_count 3, got %v", val)
	}
	if _, ok := getAttrValue(r, "first_seen"); !ok {
		t.Fatalf("expected first_seen attribute")
	}
	if _, ok := getAttrValue(r, "last_seen"); !ok {
		t.Fatalf("expected last_seen attribute")
	}
	if _, ok := getAttrValue(r, "original_msg"); ok {
		t.Fatalf("handler emission should not have original_msg attribute")
	}

	// Fourth log immediately after reset: suppressed (new window started, count=2)
	if err := handler.Handle(ctx, makeRecord(slog.LevelInfo, "reset-test", slog.String("k", "v"))); err != nil {
		t.Fatalf("handle err: %v", err)
	}
	if capture.Len() != 2 {
		t.Fatalf("expected duplicate after reset to be suppressed, got %d", capture.Len())
	}

	time.Sleep(60 * time.Millisecond)

	// Fifth log: emitted with NEW duplicate metadata (reset worked!)
	if err := handler.Handle(ctx, makeRecord(slog.LevelInfo, "reset-test", slog.String("k", "v"))); err != nil {
		t.Fatalf("handle err: %v", err)
	}
	if capture.Len() != 3 {
		t.Fatalf("expected second re-log after reset, got %d", capture.Len())
	}

	r2 := capture.Get(2)
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

	capture := &captureHandler{}
	engine := New(
		WithDuplicateLogWindow(30*time.Millisecond),
		WithCleanupInterval(1*time.Second),
		WithCacheExpiry(10*time.Second),
	)
	engine.Start(t.Context())
	defer engine.Stop()

	handler := newTestHandler(engine, capture)
	ctx := t.Context()

	for cycle := range 3 {
		expectedRecords := cycle + 1

		if cycle == 0 {
			if err := handler.Handle(ctx, makeRecord(slog.LevelInfo, "cycle-test")); err != nil {
				t.Fatalf("cycle %d: handle err: %v", cycle, err)
			}
			if capture.Len() != 1 {
				t.Fatalf("cycle %d: expected first record to pass, got %d", cycle, capture.Len())
			}
		}

		for range 2 {
			if err := handler.Handle(ctx, makeRecord(slog.LevelInfo, "cycle-test")); err != nil {
				t.Fatalf("cycle %d: handle err: %v", cycle, err)
			}
		}

		time.Sleep(40 * time.Millisecond)

		if err := handler.Handle(ctx, makeRecord(slog.LevelInfo, "cycle-test")); err != nil {
			t.Fatalf("cycle %d: handle err: %v", cycle, err)
		}

		if capture.Len() != expectedRecords+1 {
			t.Fatalf("cycle %d: expected %d records, got %d", cycle, expectedRecords+1, capture.Len())
		}

		lastRecord := capture.Get(capture.Len() - 1)
		if val, ok := getAttrValue(lastRecord, "duplicate_count"); ok {
			if val.Int64() > 5 {
				t.Fatalf("cycle %d: duplicate_count %d is too high, suggests reset not working", cycle, val.Int64())
			}
		}
	}
}

// TestCleanupEmissionsHaveOriginalMsg verifies that logs emitted via the
// background cleanup path include the original_msg attribute.
func TestCleanupEmissionsHaveOriginalMsg(t *testing.T) {
	t.Parallel()

	capture := &captureHandler{}
	engine := New(
		WithDuplicateLogWindow(100*time.Millisecond),
		WithCleanupInterval(20*time.Millisecond),
		WithCacheExpiry(50*time.Millisecond),
	)
	engine.Start(t.Context())
	defer engine.Stop()

	handler := newTestHandler(engine, capture)
	ctx := t.Context()

	if err := handler.Handle(ctx, makeRecord(slog.LevelInfo, "cleanup-test", slog.String("key", "value"))); err != nil {
		t.Fatalf("handle err: %v", err)
	}
	if err := handler.Handle(ctx, makeRecord(slog.LevelInfo, "cleanup-test", slog.String("key", "value"))); err != nil {
		t.Fatalf("handle err: %v", err)
	}

	initialCount := capture.Len()

	time.Sleep(150 * time.Millisecond)

	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) && capture.Len() <= initialCount {
		time.Sleep(10 * time.Millisecond)
	}

	if capture.Len() <= initialCount {
		t.Fatalf("expected cleanup to emit summary record")
	}

	cleanupRecord := capture.Get(capture.Len() - 1)
	if val, ok := getAttrValue(cleanupRecord, "original_msg"); !ok {
		t.Fatalf("cleanup emission should have original_msg attribute")
	} else if val.String() != "cleanup-test" {
		t.Fatalf("expected original_msg 'cleanup-test', got %q", val.String())
	}

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

// TestDifferentComponentsNotDeduped verifies that logs from different handler
// chains (e.g., different "component" values set via WithAttrs) are NOT
// deduplicated together, even when they have the same message and attrs.
func TestDifferentComponentsNotDeduped(t *testing.T) {
	t.Parallel()

	var buf1, buf2 bytes.Buffer
	handler1 := slog.NewJSONHandler(&buf1, &slog.HandlerOptions{Level: slog.LevelDebug})
	handler2 := slog.NewJSONHandler(&buf2, &slog.HandlerOptions{Level: slog.LevelDebug})

	engine := New(
		WithDuplicateLogWindow(50*time.Millisecond),
		WithCleanupInterval(1*time.Second),
		WithCacheExpiry(10*time.Second),
	)
	engine.Start(t.Context())
	defer engine.Stop()

	mw := engine.NewMiddleware()

	componentA := mw(handler1).WithAttrs([]slog.Attr{slog.String("component", "osquery")})
	componentB := mw(handler2).WithAttrs([]slog.Attr{slog.String("component", "metadata_writer")})

	ctx := t.Context()

	rec := makeRecord(slog.LevelInfo, "server metadata changed", slog.String("key", "val"))
	if err := componentA.Handle(ctx, rec); err != nil {
		t.Fatalf("componentA handle err: %v", err)
	}
	if err := componentB.Handle(ctx, rec); err != nil {
		t.Fatalf("componentB handle err: %v", err)
	}

	if buf1.Len() == 0 {
		t.Fatal("expected componentA log to pass through")
	}
	if buf2.Len() == 0 {
		t.Fatal("expected componentB log to pass through")
	}

	var data1, data2 map[string]any
	if err := json.Unmarshal(buf1.Bytes(), &data1); err != nil {
		t.Fatalf("unmarshal componentA: %v", err)
	}
	if err := json.Unmarshal(buf2.Bytes(), &data2); err != nil {
		t.Fatalf("unmarshal componentB: %v", err)
	}
	if data1["component"] != "osquery" {
		t.Fatalf("expected component 'osquery', got %v", data1["component"])
	}
	if data2["component"] != "metadata_writer" {
		t.Fatalf("expected component 'metadata_writer', got %v", data2["component"])
	}
}

// TestSameComponentDeduped verifies that logs from the same handler chain ARE
// correctly deduplicated, while logs from a different chain are not.
func TestSameComponentDeduped(t *testing.T) {
	t.Parallel()

	capture := &captureHandler{}
	engine := New(
		WithDuplicateLogWindow(50*time.Millisecond),
		WithCleanupInterval(1*time.Second),
		WithCacheExpiry(10*time.Second),
	)
	engine.Start(t.Context())
	defer engine.Stop()

	mw := engine.NewMiddleware()
	handlerA := mw(capture).WithAttrs([]slog.Attr{slog.String("component", "osquery")})
	handlerB := mw(capture).WithAttrs([]slog.Attr{slog.String("component", "metadata_writer")})

	ctx := t.Context()

	// First log from component A: passes through
	if err := handlerA.Handle(ctx, makeRecord(slog.LevelInfo, "test msg", slog.String("k", "v"))); err != nil {
		t.Fatalf("handle err: %v", err)
	}
	if capture.Len() != 1 {
		t.Fatalf("expected first record to pass, got %d", capture.Len())
	}

	// Same message from component A again: suppressed
	if err := handlerA.Handle(ctx, makeRecord(slog.LevelInfo, "test msg", slog.String("k", "v"))); err != nil {
		t.Fatalf("handle err: %v", err)
	}
	if capture.Len() != 1 {
		t.Fatalf("expected duplicate to be suppressed, got %d", capture.Len())
	}

	// Same message from component B: passes through (different handler chain)
	if err := handlerB.Handle(ctx, makeRecord(slog.LevelInfo, "test msg", slog.String("k", "v"))); err != nil {
		t.Fatalf("handle err: %v", err)
	}
	if capture.Len() != 2 {
		t.Fatalf("expected different component to pass, got %d", capture.Len())
	}
}

// TestCleanupUsesCorrectHandler verifies that background cleanup emissions use
// the per-entry downstream handler, not an arbitrary one.
func TestCleanupUsesCorrectHandler(t *testing.T) {
	t.Parallel()

	captureA := &captureHandler{}
	captureB := &captureHandler{}
	engine := New(
		WithDuplicateLogWindow(100*time.Millisecond),
		WithCleanupInterval(20*time.Millisecond),
		WithCacheExpiry(50*time.Millisecond),
	)
	engine.Start(t.Context())
	defer engine.Stop()

	mw := engine.NewMiddleware()
	handlerA := mw(captureA).WithAttrs([]slog.Attr{slog.String("component", "comp_a")})
	handlerB := mw(captureB).WithAttrs([]slog.Attr{slog.String("component", "comp_b")})

	ctx := t.Context()

	// Create duplicates for component A
	if err := handlerA.Handle(ctx, makeRecord(slog.LevelInfo, "dup msg")); err != nil {
		t.Fatalf("handle err: %v", err)
	}
	if err := handlerA.Handle(ctx, makeRecord(slog.LevelInfo, "dup msg")); err != nil {
		t.Fatalf("handle err: %v", err)
	}

	// Create duplicates for component B (same message, different handler chain)
	if err := handlerB.Handle(ctx, makeRecord(slog.LevelInfo, "dup msg")); err != nil {
		t.Fatalf("handle err: %v", err)
	}
	if err := handlerB.Handle(ctx, makeRecord(slog.LevelInfo, "dup msg")); err != nil {
		t.Fatalf("handle err: %v", err)
	}

	// Wait for cache expiry + cleanup
	time.Sleep(150 * time.Millisecond)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if captureA.Len() > 1 && captureB.Len() > 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if captureA.Len() < 2 {
		t.Fatalf("expected cleanup emission for component A, got %d records", captureA.Len())
	}
	if captureB.Len() < 2 {
		t.Fatalf("expected cleanup emission for component B, got %d records", captureB.Len())
	}
}

// TestHandlerEmissionsLackOriginalMsg verifies that logs emitted via the handler
// window-elapsed path do NOT include the original_msg attribute, distinguishing
// them from cleanup emissions.
func TestHandlerEmissionsLackOriginalMsg(t *testing.T) {
	t.Parallel()

	capture := &captureHandler{}
	engine := New(
		WithDuplicateLogWindow(30*time.Millisecond),
		WithCleanupInterval(1*time.Second),
		WithCacheExpiry(10*time.Second),
	)
	engine.Start(t.Context())
	defer engine.Stop()

	handler := newTestHandler(engine, capture)
	ctx := t.Context()

	if err := handler.Handle(ctx, makeRecord(slog.LevelInfo, "handler-test")); err != nil {
		t.Fatalf("handle err: %v", err)
	}

	if err := handler.Handle(ctx, makeRecord(slog.LevelInfo, "handler-test")); err != nil {
		t.Fatalf("handle err: %v", err)
	}

	time.Sleep(40 * time.Millisecond)

	if err := handler.Handle(ctx, makeRecord(slog.LevelInfo, "handler-test")); err != nil {
		t.Fatalf("handle err: %v", err)
	}

	if capture.Len() != 2 {
		t.Fatalf("expected 2 records, got %d", capture.Len())
	}

	emitted := capture.Get(1)
	if _, ok := getAttrValue(emitted, "original_msg"); ok {
		t.Fatalf("handler emission should NOT have original_msg attribute")
	}

	if _, ok := getAttrValue(emitted, "duplicate_count"); !ok {
		t.Fatalf("handler emission should have duplicate_count")
	}
	if _, ok := getAttrValue(emitted, "first_seen"); !ok {
		t.Fatalf("handler emission should have first_seen")
	}
	if _, ok := getAttrValue(emitted, "last_seen"); !ok {
		t.Fatalf("handler emission should have last_seen")
	}
}
