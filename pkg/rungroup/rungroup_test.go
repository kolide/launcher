package rungroup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kolide/launcher/v2/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// Longest any test here should need: waiting out both rungroup timeouts, plus slack for a slow machine
const testTimeout = InterruptTimeout + executeReturnTimeout + 5*time.Second

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestRun_NoActors(t *testing.T) {
	t.Parallel()

	testRunGroup := NewRunGroup()
	require.NoError(t, testRunGroup.Run())
}

// RunGroup should interrupt its actors on the first error, allow a slow actor to finish,
// and interrupt every actor cleanly.
func TestRun_MultipleActors(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), testTimeout)
	defer cancel()

	testRunGroup := NewRunGroup()
	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}))
	testRunGroup.SetSlogger(slogger)

	var groupInterruptCount atomic.Int32
	actorCtx, releaseActors := context.WithCancel(ctx)

	// These actors are well-behaved: wait for interrupt, exit cleanly
	for i := range 3 {
		testRunGroup.Add(fmt.Sprintf("actor%d", i), func() error {
			<-actorCtx.Done()
			return nil
		}, func(error) {
			groupInterruptCount.Add(1)
			releaseActors()
		})
	}

	// This actor interrupts the group by erroring on exit
	expectedError := errors.New("test error from interruptingActor")
	testRunGroup.Add("interruptingActor", func() error {
		return expectedError
	}, func(error) {
		groupInterruptCount.Add(1)
	})

	// This actor lags on its way out, and the group is expected to wait for it
	var slowActorExited atomic.Bool
	testRunGroup.Add("slowActor", func() error {
		<-actorCtx.Done()
		time.Sleep(time.Second) // well inside executeReturnTimeout
		slowActorExited.Store(true)
		return nil
	}, func(error) {
		groupInterruptCount.Add(1)
		releaseActors()
	})

	runCompleted := make(chan error, 1)
	go func() { runCompleted <- testRunGroup.Run() }()

	select {
	case err := <-runCompleted:
		require.ErrorIs(t, err, expectedError, "rungroup.Run didn't return interruptingActor's error")
	case <-ctx.Done():
		t.Errorf("rungroup.Run did not return in time, got %d interrupts. logs: %s", groupInterruptCount.Load(), logBytes.String())
		t.FailNow()
	}

	require.Truef(t, slowActorExited.Load(), "the slow actor was not allowed to exit cleanly: logs: %s", logBytes.String())
	require.Equalf(t, int32(5), groupInterruptCount.Load(), "unexpected number of interrupts: logs: %s", logBytes.String())
}

func TestRun_MultipleActors_InterruptTimeout(t *testing.T) {
	t.Parallel()

	testRunGroup := NewRunGroup()
	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}))
	testRunGroup.SetSlogger(slogger)

	groupReceivedInterrupts := make(chan struct{}, 3)

	// First actor waits for interrupt and alerts groupReceivedInterrupts when it's interrupted
	firstActorInterrupt := make(chan struct{})
	testRunGroup.Add("firstActor", func() error {
		<-firstActorInterrupt
		return nil
	}, func(error) {
		groupReceivedInterrupts <- struct{}{}
		firstActorInterrupt <- struct{}{}
	})

	// Second actor returns error on `execute`, and then alerts groupReceivedInterrupts when it's interrupted
	expectedError := errors.New("test error from interruptingActor")
	testRunGroup.Add("interruptingActor", func() error {
		time.Sleep(1 * time.Second)
		return expectedError
	}, func(error) {
		groupReceivedInterrupts <- struct{}{}
	})

	// Third actor blocks in interrupt for longer than the interrupt timeout
	blockingActorInterrupt := make(chan struct{})
	testRunGroup.Add("blockingActor", func() error {
		<-blockingActorInterrupt
		return nil
	}, func(error) {
		time.Sleep(4 * InterruptTimeout)
		groupReceivedInterrupts <- struct{}{}
		blockingActorInterrupt <- struct{}{}
	})

	runCompleted := make(chan struct{})
	go func() {
		err := testRunGroup.Run()
		require.Error(t, err)
		runCompleted <- struct{}{}
	}()

	// 1 second before interrupt, waiting for interrupt, and waiting for execute return, plus a little buffer
	runDuration := 1*time.Second + InterruptTimeout + executeReturnTimeout + 1*time.Second
	interruptCheckTimer := time.NewTicker(runDuration)
	defer interruptCheckTimer.Stop()

	receivedInterrupts := 0
	gotRunCompleted := false
	for !gotRunCompleted {
		select {
		case <-groupReceivedInterrupts:
			receivedInterrupts += 1
		case <-runCompleted:
			gotRunCompleted = true
		case <-interruptCheckTimer.C:
			t.Errorf("did not receive expected interrupts within reasonable time, got %d", receivedInterrupts)
			t.FailNow()
		}
	}

	require.True(t, gotRunCompleted, "rungroup.Run did not terminate within time limit")

	// We only want two interrupts -- we should not be waiting on the blocking actor
	require.Equal(t, 2, receivedInterrupts, "unexpected number of interrupts: logs:", logBytes.String())

	// Wait for all goroutines to exit
	time.Sleep(4 * InterruptTimeout)
}

func TestRun_MultipleActors_ExecuteReturnTimeout(t *testing.T) {
	t.Parallel()

	testRunGroup := NewRunGroup()
	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}))
	testRunGroup.SetSlogger(slogger)

	groupReceivedInterrupts := make(chan struct{}, 3)
	// Keep track of when `execute`s return so we give testRunGroup.Run enough time to do its thing
	groupReceivedExecuteReturns := make(chan struct{}, 2)

	// First actor waits for interrupt and alerts groupReceivedInterrupts when it's interrupted
	firstActorInterrupt := make(chan struct{})
	testRunGroup.Add("firstActor", func() error {
		<-firstActorInterrupt
		groupReceivedExecuteReturns <- struct{}{}
		return nil
	}, func(error) {
		groupReceivedInterrupts <- struct{}{}
		firstActorInterrupt <- struct{}{}
	})

	// Second actor returns error on `execute`, and then alerts groupReceivedInterrupts when it's interrupted
	expectedError := errors.New("test error from interruptingActor")
	testRunGroup.Add("interruptingActor", func() error {
		time.Sleep(1 * time.Second)
		groupReceivedExecuteReturns <- struct{}{}
		return expectedError
	}, func(error) {
		groupReceivedInterrupts <- struct{}{}
	})

	// Third actor never signals to `execute` to return
	blockingActorInterrupt := make(chan struct{})
	testRunGroup.Add("blockingActor", func() error {
		<-blockingActorInterrupt                  // will never happen
		groupReceivedExecuteReturns <- struct{}{} // will never happen
		return nil
	}, func(error) {
		groupReceivedInterrupts <- struct{}{}
	})

	runCompleted := make(chan struct{})
	go func() {
		err := testRunGroup.Run()
		runCompleted <- struct{}{}
		require.Error(t, err)
	}()

	// 1 second before interrupt, waiting for interrupt, and waiting for execute return, plus a little buffer
	runDuration := 1*time.Second + InterruptTimeout + executeReturnTimeout + 1*time.Second
	interruptCheckTimer := time.NewTicker(runDuration)
	defer interruptCheckTimer.Stop()

	// Make sure all three actors are interrupted, and that two of them terminate their execute
	receivedInterrupts := 0
	receivedExecuteReturns := 0
	gotRunCompleted := false
	for !gotRunCompleted {
		select {
		case <-groupReceivedInterrupts:
			receivedInterrupts += 1
		case <-groupReceivedExecuteReturns:
			receivedExecuteReturns += 1
		case <-runCompleted:
			gotRunCompleted = true
		case <-interruptCheckTimer.C:
			t.Errorf("did not receive expected interrupts within reasonable time, got %d", receivedInterrupts)
			t.FailNow()
		}
	}

	require.True(t, gotRunCompleted, "rungroup.Run did not terminate within time limit")
	require.Equal(t, 3, receivedInterrupts, "unexpected number of interrupts: logs:", logBytes.String())
	require.Equal(t, 2, receivedExecuteReturns)

	// Clean up goroutine
	blockingActorInterrupt <- struct{}{}
}

func TestRun_RecoversAndLogsPanic(t *testing.T) {
	t.Parallel()

	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	testRunGroup := NewRunGroup()
	testRunGroup.SetSlogger(slogger)

	// Actor that will panic in its execute function
	testRunGroup.Add("panickingActor", func() error {
		time.Sleep(1 * time.Second)
		panic("test panic in rungroup actor") //nolint:forbidigo // Fine to use panic in tests
	}, func(error) {})

	runCompleted := make(chan struct{})
	go func() {
		err := testRunGroup.Run()
		runCompleted <- struct{}{}
		require.Error(t, err)
	}()

	// Give it a bit of time to return
	runDuration := 1*time.Second + InterruptTimeout + executeReturnTimeout + 1*time.Second
	interruptCheckTimer := time.NewTicker(runDuration)
	defer interruptCheckTimer.Stop()

	// Confirm that the rungroup exited without panicking (i.e. we recovered appropriately)
	gotRunCompleted := false
	for !gotRunCompleted {
		select {
		case <-runCompleted:
			gotRunCompleted = true
		case <-interruptCheckTimer.C:
			fmt.Println(logBytes.String())
			t.Error("did not interrupt within reasonable time")
			t.FailNow()
		}
	}
	require.True(t, gotRunCompleted, "rungroup.Run did not terminate within time limit")

	// Confirm we have some sort of log about the panic
	require.Contains(t, logBytes.String(), "panic")
}
