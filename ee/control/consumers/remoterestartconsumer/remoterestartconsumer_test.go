package remoterestartconsumer

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/kolide/kit/ulid"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
)

func TestDo(t *testing.T) {
	t.Parallel()

	currentRunId := ulid.New()

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())
	mockKnapsack.On("GetRunID").Return(currentRunId)

	remoteRestarter := New(mockKnapsack)

	testAction := remoteRestartAction{
		RunID: currentRunId,
	}
	testActionRaw, err := json.Marshal(testAction)
	require.NoError(t, err)

	// We don't expect an error because we should process the action
	require.NoError(t, remoteRestarter.Do(bytes.NewReader(testActionRaw)), "expected no error processing valid remote restart action")

	// The restarter should delay before sending an error to `signalRestart`
	require.Len(t, remoteRestarter.signalRestart, 0, "expected restarter to delay before signal for restart but channel is already has item in it")
	time.Sleep(restartDelay + 2*time.Second)
	require.Len(t, remoteRestarter.signalRestart, 1, "expected restarter to signal for restart but channel is empty after delay")
}

func TestDo_DoesNotSignalRestartWhenRunIDDoesNotMatch(t *testing.T) {
	t.Parallel()

	currentRunId := ulid.New()

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())
	mockKnapsack.On("GetRunID").Return(currentRunId)

	remoteRestarter := New(mockKnapsack)

	testAction := remoteRestartAction{
		RunID: ulid.New(), // run ID will not match `currentRunId`
	}
	testActionRaw, err := json.Marshal(testAction)
	require.NoError(t, err)

	// We don't expect an error because we want to discard this action
	require.NoError(t, remoteRestarter.Do(bytes.NewReader(testActionRaw)), "should not return error for old run ID")

	// The restarter should not send an error to `signalRestart`
	time.Sleep(restartDelay + 2*time.Second)
	require.Len(t, remoteRestarter.signalRestart, 0, "restarter should not have signaled for a restart, but channel is not empty")
}

func TestDo_DoesNotSignalRestartWhenRunIDIsEmpty(t *testing.T) {
	t.Parallel()

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())

	remoteRestarter := New(mockKnapsack)

	testAction := remoteRestartAction{
		RunID: "", // run ID is empty
	}
	testActionRaw, err := json.Marshal(testAction)
	require.NoError(t, err)

	// We don't expect an error because we want to discard this action
	require.NoError(t, remoteRestarter.Do(bytes.NewReader(testActionRaw)), "should not return error for empty run ID")

	// The restarter should not send an error to `signalRestart`
	time.Sleep(restartDelay + 2*time.Second)
	require.Len(t, remoteRestarter.signalRestart, 0, "restarter should not have signaled for a restart, but channel is not empty")
}

func TestDo_DoesNotRestartIfInterruptedDuringDelay(t *testing.T) {
	t.Parallel()

	currentRunId := ulid.New()

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())
	mockKnapsack.On("GetRunID").Return(currentRunId)

	remoteRestarter := New(mockKnapsack)

	testAction := remoteRestartAction{
		RunID: currentRunId,
	}
	testActionRaw, err := json.Marshal(testAction)
	require.NoError(t, err)

	// We don't expect an error because the run ID is correct
	require.NoError(t, remoteRestarter.Do(bytes.NewReader(testActionRaw)), "expected no error processing valid remote restart action")

	// The restarter should delay before sending an error to `signalRestart`
	require.Len(t, remoteRestarter.signalRestart, 0, "expected restarter to delay before signal for restart but channel is already has item in it")

	// Now, send an interrupt
	remoteRestarter.Interrupt(errors.New("test error"))

	// Sleep beyond the interrupt delay, and confirm we don't try to do a restart when we're already shutting down
	time.Sleep(restartDelay + 2*time.Second)
	require.Len(t, remoteRestarter.signalRestart, 0, "restarter should not have tried to signal for restart when interrupted during restart delay")
}

func TestInterrupt_Multiple(t *testing.T) {
	t.Parallel()

	mockKnapsack := typesmocks.NewKnapsack(t)
	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	mockKnapsack.On("Slogger").Return(slogger)

	remoteRestarter := New(mockKnapsack)

	// Let the remote restarter run for a bit
	go remoteRestarter.Execute()
	time.Sleep(3 * time.Second)
	interruptStart := time.Now()
	remoteRestarter.Interrupt(errors.New("test error"))

	// Confirm we can call Interrupt multiple times without blocking
	interruptComplete := make(chan struct{})
	expectedInterrupts := 3
	for i := 0; i < expectedInterrupts; i += 1 {
		go func() {
			remoteRestarter.Interrupt(nil)
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
