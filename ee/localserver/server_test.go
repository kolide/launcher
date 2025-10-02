package localserver

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestInterrupt_Multiple(t *testing.T) {
	t.Parallel()

	k := typesmocks.NewKnapsack(t)
	k.On("KolideServerURL").Return("localserver")
	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	k.On("Slogger").Return(slogger)
	k.On("Registrations").Return([]types.Registration{}, nil) // return empty set of registrations so we will get a munemo worker
	k.On("LatestOsquerydPath", mock.Anything).Return("")

	// Override the poll and recalculate interval for the test so we can be sure that the async workers
	// do run, but then stop running on shutdown
	pollInterval = 2 * time.Second
	recalculateInterval = 100 * time.Millisecond

	// Create the localserver
	ls, err := New(t.Context(), k, nil)
	require.NoError(t, err)

	// Let the server run for a bit
	go ls.Start()
	time.Sleep(3 * time.Second)
	interruptStart := time.Now()
	ls.Interrupt(errors.New("test error"))

	// Confirm we can call Interrupt multiple times without blocking
	interruptComplete := make(chan struct{})
	expectedInterrupts := 3
	for i := 0; i < expectedInterrupts; i += 1 {
		go func() {
			ls.Interrupt(nil)
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

	// Confirm all workers shut down. Checking against logs can be a little flaky in CI, so we sleep a couple seconds first
	// just to be safe.
	time.Sleep(3 * time.Second)
	logs := logBytes.String()
	require.Contains(t, logs, "runAsyncdWorkers received shutdown signal", "id fields worker did not shut down")
	require.Contains(t, logs, "getMunemoFromKnapsack received shutdown signal", "munemo worker did not shut down")
	require.Contains(t, logs, "callback worker shut down", "middleware callback worker did not shut down")

	k.AssertExpectations(t)
}
