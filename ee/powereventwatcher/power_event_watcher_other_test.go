//go:build !windows
// +build !windows

package powereventwatcher

import (
	"context"
	"errors"
	"testing"
	"time"

	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func TestInterrupt_Multiple(t *testing.T) {
	t.Parallel()

	ksubscriber := NewKnapsackSleepStateUpdater(multislogger.NewNopLogger(), typesmocks.NewKnapsack(t))
	p, err := New(context.TODO(), multislogger.NewNopLogger(), ksubscriber)
	require.NoError(t, err)

	// Start and then interrupt
	go p.Execute()
	p.Interrupt(errors.New("test error"))

	// Confirm we can call Interrupt multiple times without blocking
	interruptComplete := make(chan struct{})
	expectedInterrupts := 3
	for i := 0; i < expectedInterrupts; i += 1 {
		go func() {
			p.Interrupt(nil)
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
			t.Errorf("could not call interrupt multiple times and return within 5 seconds -- received %d interrupts before timeout", receivedInterrupts)
			t.FailNow()
		}
	}

	require.Equal(t, expectedInterrupts, receivedInterrupts)
}
