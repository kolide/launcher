package gowrapper

import (
	"log/slog"
	"testing"
	"time"

	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
)

func TestGo_WithPanic(t *testing.T) {
	t.Parallel()

	funcDelay := 200 * time.Millisecond

	// Capture goroutine results so we know the goroutine executed
	goroutineResults := make(chan struct{})
	g := func() {
		time.Sleep(funcDelay)
		goroutineResults <- struct{}{}
		panic("test panic") //nolint:forbidigo // Fine to use panic in tests
	}

	// Capture onPanic results so we know that onPanic executed
	onPanicResults := make(chan struct{})
	p := func(_ any) {
		time.Sleep(funcDelay)
		onPanicResults <- struct{}{}
	}

	// Capture slogger output so we confirm we do some logging
	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Kick off goroutine
	GoWithRecoveryAction(t.Context(), slogger, g, p)
	timeoutGracePeriod := 200 * time.Millisecond

	goroutineExecuted := false
	onPanicExecuted := false

	for {
		// Goroutine all done
		if goroutineExecuted && onPanicExecuted {
			break
		}

		select {
		case <-goroutineResults:
			goroutineExecuted = true
		case <-onPanicResults:
			onPanicExecuted = true
		case <-time.After(funcDelay + funcDelay + timeoutGracePeriod):
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

	funcDelay := 200 * time.Millisecond

	// Capture goroutine results so we know the goroutine executed
	goroutineResults := make(chan struct{})
	g := func() {
		time.Sleep(funcDelay)
		goroutineResults <- struct{}{}
	}

	// Capture onPanic results so we know that onPanic did not execute
	onPanicResults := make(chan struct{})
	p := func(_ any) {
		time.Sleep(funcDelay)
		onPanicResults <- struct{}{}
	}

	// Kick off goroutine
	GoWithRecoveryAction(t.Context(), multislogger.NewNopLogger(), g, p)
	timeoutGracePeriod := 200 * time.Millisecond
	goroutineEndTime := time.Now().Add(funcDelay + funcDelay + timeoutGracePeriod)

	goroutineExecuted := false
	onPanicExecuted := false

	for {
		// Wait until our timeout to make sure onPanic won't execute
		if time.Now().After(goroutineEndTime) {
			break
		}

		select {
		case <-goroutineResults:
			goroutineExecuted = true
		case <-onPanicResults:
			onPanicExecuted = true
		case <-time.After(funcDelay + funcDelay + timeoutGracePeriod):
			continue
		}
	}

	require.True(t, goroutineExecuted, "goroutine did not execute")
	require.False(t, onPanicExecuted, "onPanic should not have executed")
}
