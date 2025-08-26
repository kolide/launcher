package multislogger

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/types/mocks"
)

func TestMultiSlogger_FlagsChanged_DuplicateLogWindow(t *testing.T) {
	t.Parallel()

	// Create a multislogger with dedup enabled
	ms := NewWithDedup(50 * time.Millisecond)
	ms.Start(context.Background())
	defer ms.Stop()

	// Create a mock flags interface
	mockFlags := mocks.NewFlags(t)
	mockFlags.On("DuplicateLogWindow").Return(100 * time.Millisecond)

	// Set the flags on the multislogger
	ms.SetFlags(mockFlags)

	// Call FlagsChanged with DuplicateLogWindow flag
	ctx := context.Background()
	ms.FlagsChanged(ctx, keys.DuplicateLogWindow)

	// Verify that the mock was called
	mockFlags.AssertCalled(t, "DuplicateLogWindow")

	// Verify that the dedup engine was updated by checking its behavior
	// (This is an indirect test since getDuplicateLogWindow is not exported from the engine)
	// We'll test by verifying dedup behavior changes
}

func TestMultiSlogger_FlagsChanged_OtherFlags(t *testing.T) {
	t.Parallel()

	// Create a multislogger with dedup enabled
	ms := NewWithDedup(50 * time.Millisecond)
	ms.Start(context.Background())
	defer ms.Stop()

	// Create a mock flags interface
	mockFlags := mocks.NewFlags(t)

	// Set the flags on the multislogger
	ms.SetFlags(mockFlags)

	// Call FlagsChanged with non-DuplicateLogWindow flags
	ctx := context.Background()
	ms.FlagsChanged(ctx, keys.Debug, keys.LoggingInterval)

	// Verify that DuplicateLogWindow() was NOT called since we didn't pass that flag
	mockFlags.AssertNotCalled(t, "DuplicateLogWindow")
}

func TestMultiSlogger_FlagsChanged_NilFlags(t *testing.T) {
	t.Parallel()

	// Create a multislogger without setting flags
	ms := NewWithDedup(50 * time.Millisecond)
	ms.Start(context.Background())
	defer ms.Stop()

	// Call FlagsChanged without setting flags - should not panic
	ctx := context.Background()
	ms.FlagsChanged(ctx, keys.DuplicateLogWindow)
	// Test passes if no panic occurs
}

func TestMultiSlogger_SetFlags(t *testing.T) {
	t.Parallel()

	ms := NewWithDedup(50 * time.Millisecond)
	mockFlags := mocks.NewFlags(t)

	// Test setting flags
	ms.SetFlags(mockFlags)

	// Verify flags are set by testing FlagsChanged behavior
	mockFlags.On("DuplicateLogWindow").Return(100 * time.Millisecond)

	ctx := context.Background()
	ms.FlagsChanged(ctx, keys.DuplicateLogWindow)

	mockFlags.AssertCalled(t, "DuplicateLogWindow")
}

func TestMultiSlogger_SetFlags_Nil(t *testing.T) {
	t.Parallel()

	// Test setting nil multislogger - should not panic
	var nilMs *MultiSlogger
	nilMs.SetFlags(nil)
	// Test passes if no panic occurs
}

func TestMultiSlogger_FlagsChanged_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	// Create a multislogger with dedup enabled
	ms := NewWithDedup(50 * time.Millisecond)
	ms.Start(context.Background())
	defer ms.Stop()

	// Create a mock flags interface with thread-safe mock
	mockFlags := &mocks.Flags{}
	mockFlags.On("DuplicateLogWindow").Return(100 * time.Millisecond).Maybe()

	// Set the flags on the multislogger
	ms.SetFlags(mockFlags)

	// Start multiple goroutines that concurrently call FlagsChanged
	var wg sync.WaitGroup
	done := make(chan struct{})

	// Goroutine 1: Continuously trigger flag changes
	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx := context.Background()
		for {
			select {
			case <-done:
				return
			default:
				ms.FlagsChanged(ctx, keys.DuplicateLogWindow)
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()

	// Goroutine 2: Continuously update duplicate log window directly
	wg.Add(1)
	go func() {
		defer wg.Done()
		windows := []time.Duration{0, 50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond}
		i := 0
		for {
			select {
			case <-done:
				return
			default:
				ms.UpdateDuplicateLogWindow(windows[i%len(windows)])
				i++
				time.Sleep(15 * time.Millisecond)
			}
		}
	}()

	// Goroutine 3: Continuously process logs to exercise the dedup engine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			select {
			case <-done:
				return
			default:
				// Create a test log record and process it through the multislogger
				ms.Log(context.Background(), slog.LevelInfo, "concurrent test", "iteration", i)
				time.Sleep(5 * time.Millisecond)
			}
		}
	}()

	// Let the test run for a short duration
	time.Sleep(300 * time.Millisecond)
	close(done)
	wg.Wait()

	// Test passes if no race conditions occurred (detected by go test -race)
}

func TestMultiSlogger_UpdateDuplicateLogWindow(t *testing.T) {
	t.Parallel()

	ms := NewWithDedup(50 * time.Millisecond)
	ms.Start(context.Background())
	defer ms.Stop()

	// Test updating the window
	ms.UpdateDuplicateLogWindow(100 * time.Millisecond)

	// We can't directly verify the internal state, but we can verify the method doesn't panic
	// and that the multislogger continues to function
	ms.Log(context.Background(), slog.LevelInfo, "test message after update")
}

func TestMultiSlogger_UpdateDuplicateLogWindow_NilEngine(t *testing.T) {
	t.Parallel()

	// Create a multislogger without dedup (nil engine)
	ms := New()

	// Test updating window on nil engine - should not panic
	ms.UpdateDuplicateLogWindow(100 * time.Millisecond)
	// Test passes if no panic occurs
}

func TestMultiSlogger_UpdateDuplicateLogWindow_NilMultiSlogger(t *testing.T) {
	t.Parallel()

	// Test updating window on nil multislogger - should not panic
	var nilMs *MultiSlogger
	nilMs.UpdateDuplicateLogWindow(100 * time.Millisecond)
	// Test passes if no panic occurs
}
