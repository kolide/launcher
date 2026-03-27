package gowrapper

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/kolide/launcher/v2/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestGo_WithPanic(t *testing.T) {
	t.Parallel()

	// Capture goroutine results so we know the goroutine executed
	goroutineResults := make(chan struct{})
	g := func() {
		goroutineResults <- struct{}{}
		panic("test panic") //nolint:forbidigo // Fine to use panic in tests
	}

	// Capture onPanic results so we know that onPanic executed
	onPanicResults := make(chan struct{})
	p := func(_ any) {
		onPanicResults <- struct{}{}
	}

	// Capture slogger output so we confirm we do some logging
	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Kick off goroutine
	GoWithRecoveryAction(t.Context(), slogger, g, p)
	timeoutCtx, timeoutCancel := context.WithTimeout(t.Context(), 400*time.Millisecond)
	defer timeoutCancel()

	goroutineExecuted := false
	onPanicExecuted := false

	for !(goroutineExecuted && onPanicExecuted) {
		select {
		case <-goroutineResults:
			goroutineExecuted = true
		case <-onPanicResults:
			onPanicExecuted = true
		case <-timeoutCtx.Done():
			t.Error("goroutine did not exit within timeout: logs: ", logBytes.String())
			t.FailNow()
		}
	}

	require.True(t, goroutineExecuted, "goroutine did not execute")
	require.True(t, onPanicExecuted, "onPanic did not execute")
	require.Contains(t, logBytes.String(), "panic", "logs did not include panic information")
}

func TestGo_WithoutPanic(t *testing.T) {
	t.Parallel()

	// Capture goroutine results so we know the goroutine executed
	goroutineResults := make(chan struct{})
	g := func() {
		goroutineResults <- struct{}{}
	}

	// Capture onPanic results so we know that onPanic did not execute
	onPanicResults := make(chan struct{})
	p := func(_ any) {
		onPanicResults <- struct{}{}
	}

	// Kick off goroutine
	GoWithRecoveryAction(t.Context(), multislogger.NewNopLogger(), g, p)
	recheckInterval := 100 * time.Millisecond
	timeoutGracePeriod := 400 * time.Millisecond
	goroutineEndTime := time.Now().Add(timeoutGracePeriod)

	goroutineExecuted := false
	onPanicExecuted := false

	for !(time.Now().After(goroutineEndTime)) {
		select {
		case <-goroutineResults:
			goroutineExecuted = true
		case <-onPanicResults:
			onPanicExecuted = true
		case <-time.After(recheckInterval):
			continue
		}
	}

	require.True(t, goroutineExecuted, "goroutine did not execute")
	require.False(t, onPanicExecuted, "onPanic should not have executed")
}
