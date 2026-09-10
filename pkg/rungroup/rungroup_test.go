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

// RunGroup should not wait forever on an actor's interrupt if it hangs.
func TestRun_MultipleActors_InterruptTimeout(t *testing.T) {
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
	firstCtx, releaseFirstActor := context.WithCancel(ctx)

	// This actor is well-behaved: waits for interrupt, exits cleanly
	testRunGroup.Add("firstActor", func() error {
		<-firstCtx.Done()
		return nil
	}, func(error) {
		groupInterruptCount.Add(1)
		releaseFirstActor()
	})

	// This actor interrupts the group by erroring on exit
	expectedError := errors.New("test error from interruptingActor")
	testRunGroup.Add("interruptingActor", func() error {
		return expectedError
	}, func(error) {
		groupInterruptCount.Add(1)
	})

	// Poorly behaving actor whose interrupt never returns.
	testRunGroup.Add("blockingActor", func() error {
		<-ctx.Done()
		return nil
	}, func(error) {
		<-ctx.Done()
	})

	runCompleted := make(chan error, 1)
	go func() { runCompleted <- testRunGroup.Run() }()

	select {
	case err := <-runCompleted:
		require.ErrorIs(t, err, expectedError, "rungroup.Run didn't return interruptingActor's error")
	case <-ctx.Done():
		t.Errorf("rungroup.Run did not return before timeout, got %d interrupts. logs: %s", groupInterruptCount.Load(), logBytes.String())
		t.FailNow()
	}

	// Other interrupts should fire.
	require.Equalf(t, int32(2), groupInterruptCount.Load(), "unexpected number of interrupts: logs: %s", logBytes.String())
}

// RunGroup eventually abandons a RunGroup whose actor never returns.
func TestRun_MultipleActors_ExecuteReturnTimeout(t *testing.T) {
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

	var groupInterruptCount, groupExecuteReturnCount atomic.Int32
	firstCtx, releaseFirstActor := context.WithCancel(ctx)

	// This actor is well-behaved: waits for interrupt, exits cleanly
	testRunGroup.Add("firstActor", func() error {
		defer groupExecuteReturnCount.Add(1)
		<-firstCtx.Done()
		return nil
	}, func(error) {
		groupInterruptCount.Add(1)
		releaseFirstActor()
	})

	// This actor interrupts the group by erroring on exit
	expectedError := errors.New("test error from interruptingActor")
	testRunGroup.Add("interruptingActor", func() error {
		defer groupExecuteReturnCount.Add(1)
		return expectedError
	}, func(error) {
		groupInterruptCount.Add(1)
	})

	// This actor's execute never returns
	testRunGroup.Add("blockingActor", func() error {
		defer groupExecuteReturnCount.Add(1)
		<-ctx.Done()
		return nil
	}, func(error) {
		groupInterruptCount.Add(1)
	})

	runCompleted := make(chan error, 1)
	go func() { runCompleted <- testRunGroup.Run() }()

	select {
	case err := <-runCompleted:
		require.ErrorIs(t, err, expectedError, "rungroup.Run didn't return interruptingActor's error")
	case <-ctx.Done():
		t.Errorf("rungroup.Run did not return before timeout, got %d execute returns. logs: %s", groupExecuteReturnCount.Load(), logBytes.String())
		t.FailNow()
	}

	// All three are interrupted, but only the two that can return from execute do so
	require.Equalf(t, int32(3), groupInterruptCount.Load(), "unexpected number of interrupts: logs: %s", logBytes.String())
	require.Equalf(t, int32(2), groupExecuteReturnCount.Load(), "unexpected number of execute returns: logs: %s", logBytes.String())
}

// RunGroup should handle a panic, end the group, and bubble up the error.
func TestRun_RecoversAndLogsPanic(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), testTimeout)
	defer cancel()

	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	testRunGroup := NewRunGroup()
	testRunGroup.SetSlogger(slogger)

	// Actor that will panic in its execute function
	testRunGroup.Add("panickingActor", func() error {
		panic("test panic in rungroup actor") //nolint:forbidigo // Fine to use panic in tests
	}, func(error) {})

	runCompleted := make(chan error, 1)
	go func() { runCompleted <- testRunGroup.Run() }()

	// Confirm that the rungroup exited without panicking (i.e. we recovered appropriately)
	select {
	case err := <-runCompleted:
		require.Error(t, err, "rungroup.Run didn't return the panicking actor's error")
	case <-ctx.Done():
		t.Errorf("rungroup.Run did not return after the actor panicked. logs: %s", logBytes.String())
		t.FailNow()
	}

	// Confirm we have some sort of log about the panic
	require.Contains(t, logBytes.String(), "shutting down after actor panic")
}
