package rungroup

import (
	"errors"
	"testing"
	"time"

	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func TestRun_NoActors(t *testing.T) {
	t.Parallel()

	testRunGroup := NewRunGroup(multislogger.NewNopLogger())
	require.NoError(t, testRunGroup.Run())
}

func TestRun_MultipleActors(t *testing.T) {
	t.Parallel()

	testRunGroup := NewRunGroup(multislogger.NewNopLogger())

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
		require.Error(t, err)
	}()

	// 1 second before interrupt, waiting for interrupt, and waiting for execute return, plus a little buffer
	runDuration := 1*time.Second + interruptTimeout + executeReturnTimeout + 1*time.Second
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

	require.Equal(t, 3, receivedInterrupts)
}

func TestRun_MultipleActors_InterruptTimeout(t *testing.T) {
	t.Parallel()

	testRunGroup := NewRunGroup(multislogger.NewNopLogger())

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
		time.Sleep(4 * interruptTimeout)
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
	runDuration := 1*time.Second + interruptTimeout + executeReturnTimeout + 1*time.Second
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
	require.Equal(t, 2, receivedInterrupts)
}

func TestRun_MultipleActors_ExecuteReturnTimeout(t *testing.T) {
	t.Parallel()

	testRunGroup := NewRunGroup(multislogger.NewNopLogger())

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
	runDuration := 1*time.Second + interruptTimeout + executeReturnTimeout + 1*time.Second
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
	require.Equal(t, 3, receivedInterrupts)
	require.Equal(t, 2, receivedExecuteReturns)
}
