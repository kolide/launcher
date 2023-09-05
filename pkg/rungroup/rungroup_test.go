package rungroup

import (
	"errors"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"
)

func TestRun_NoActors(t *testing.T) {
	t.Parallel()

	testRunGroup := NewRunGroup(log.NewNopLogger())
	require.NoError(t, testRunGroup.Run())
}

func TestRun_MultipleActors(t *testing.T) {
	t.Parallel()

	testRunGroup := NewRunGroup(log.NewNopLogger())

	groupReceivedInterrupts := make(chan struct{})

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

	go func() {
		err := testRunGroup.Run()
		require.Error(t, err)
	}()

	receivedInterrupts := 0
	for {
		if receivedInterrupts >= 3 {
			break
		}
		select {
		case <-groupReceivedInterrupts:
			receivedInterrupts += 1
		case <-time.After(3 * time.Second):
			t.Error("did not receive expected interrupts within reasonable time")
		}
	}

	require.Equal(t, 3, receivedInterrupts)
}
