package listener

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
)

func Test_initPipe(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()

	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("RootDirectory").Return(rootDir).Maybe()

	testListener := NewLauncherListener(mockKnapsack, slogger, "test")
	netListener, err := testListener.initPipe()
	require.NoError(t, err)
	require.NotNil(t, netListener)
	require.NoError(t, netListener.Close())
}

func TestInterrupt_Multiple(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()

	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("RootDirectory").Return(rootDir).Maybe()

	testListener := NewLauncherListener(mockKnapsack, slogger, "test")

	// Start and then interrupt
	go testListener.Execute()
	time.Sleep(3 * time.Second)
	interruptStart := time.Now()
	testListener.Interrupt(errors.New("test error"))

	// Confirm we can call Interrupt multiple times without blocking
	interruptComplete := make(chan struct{})
	expectedInterrupts := 3
	for i := 0; i < expectedInterrupts; i += 1 {
		go func() {
			testListener.Interrupt(nil)
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
