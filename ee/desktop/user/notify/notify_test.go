package notify

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/kolide/launcher/v2/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// TestInterrupt_Multiple confirms that Interrupt can be called multiple times without blocking;
// we require this for rungroup actors.
func TestInterrupt_Multiple(t *testing.T) {
	t.Parallel()

	// Set up dependencies
	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	testNotifier := NewDesktopNotifier(slogger, "", "")

	// Start and then interrupt
	go testNotifier.Execute()
	time.Sleep(3 * time.Second)
	interruptStart := time.Now()
	testNotifier.Interrupt(errors.New("test error"))

	// Confirm we can call Interrupt multiple times without blocking
	interruptComplete := make(chan struct{})
	expectedInterrupts := 3
	for range expectedInterrupts {
		go func() {
			testNotifier.Interrupt(nil)
			interruptComplete <- struct{}{}
		}()
	}

	receivedInterrupts := 0
	for receivedInterrupts < expectedInterrupts {
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
