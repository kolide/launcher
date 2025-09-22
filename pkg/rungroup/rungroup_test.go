package rungroup

import (
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
)

func TestRun_NoActors(t *testing.T) {
	t.Parallel()

	testRunGroup := NewRunGroup()
	require.NoError(t, testRunGroup.Run())
}

func TestRun_MultipleActors(t *testing.T) {
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
	expectedRuntimeForRungroup := 1 * time.Second
	expectedError := errors.New("test error from interruptingActor")
	testRunGroup.Add("interruptingActor", func() error {
		time.Sleep(expectedRuntimeForRungroup)
		return expectedError
	}, func(error) {
		groupReceivedInterrupts <- struct{}{}
	})

	// Third actor waits for interrupt and alerts groupReceivedInterrupts when it's interrupted
	anotherActorInterrupt := make(chan struct{})
	testRunGroup.Add("anotherActor", func() error {
		<-anotherActorInterrupt
		return nil
	}, func(error) {
		groupReceivedInterrupts <- struct{}{}
		anotherActorInterrupt <- struct{}{}
	})

	runCompleted := make(chan struct{})
	go func() {
		err := testRunGroup.Run()
		runCompleted <- struct{}{}
		require.Error(t, err, "run group expected to return interruptingActor's error, but did not")
	}()

	// Running until interrupt, waiting for interrupt, and waiting for execute return, plus a little buffer
	runDuration := expectedRuntimeForRungroup + InterruptTimeout + executeReturnTimeout + 1*time.Second
	interruptCheckTimer := time.NewTicker(runDuration)
	defer interruptCheckTimer.Stop()

	receivedInterrupts := 0
	gotRunCompleted := false
	for {
		if gotRunCompleted {
			break
		}

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

	require.Equal(t, 3, receivedInterrupts, "unexpected number of interrupts: logs:", logBytes.String())
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
	for {
		if gotRunCompleted {
			break
		}

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
	for {
		if gotRunCompleted {
			break
		}

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
	for {
		if gotRunCompleted {
			break
		}

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
