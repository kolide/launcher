package windowsupdatetable

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestInterrupt_Multiple(t *testing.T) {
	t.Parallel()

	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	testStore, err := storageci.NewStore(t, slogger, "test_cache_bucket")
	require.NoError(t, err)
	testFlags := typesmocks.NewFlags(t)
	testFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Maybe().Return()
	testFlags.On("InModernStandby").Maybe().Return(false)

	cacher := NewWindowsUpdatesCacher(testFlags, testStore, 1*time.Minute, slogger)

	// Start and then interrupt
	go cacher.Execute()
	time.Sleep(3 * time.Second)
	interruptStart := time.Now()
	cacher.Interrupt(errors.New("test error"))

	// Confirm we can call Interrupt multiple times without blocking
	interruptComplete := make(chan struct{})
	expectedInterrupts := 3
	for i := 0; i < expectedInterrupts; i += 1 {
		go func() {
			cacher.Interrupt(nil)
			interruptComplete <- struct{}{}
		}()
	}

	receivedInterrupts := 0
	for {
		if receivedInterrupts >= expectedInterrupts {
			break
		}

		select {
		case <-interruptComplete:
			receivedInterrupts += 1
			continue
		case <-time.After(5 * time.Second):
			t.Errorf("could not call interrupt multiple times and return within 5 seconds -- interrupted at %s, received %d interrupts before timeout; logs: \n%s\n", interruptStart.String(), receivedInterrupts, logBytes.String())
			t.FailNow()
		}
	}

	require.Equal(t, expectedInterrupts, receivedInterrupts)
}
