package localserver

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/ee/localserver/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestInterrupt_Multiple(t *testing.T) {
	t.Parallel()

	c, err := storageci.NewStore(t, multislogger.New().Logger, storage.ConfigStore.String())
	require.NoError(t, err)
	require.NoError(t, osquery.SetupLauncherKeys(c))

	k := typesmocks.NewKnapsack(t)
	k.On("KolideServerURL").Return("localserver")
	k.On("ConfigStore").Return(c)
	k.On("Slogger").Return(multislogger.New().Logger)

	// Override the poll and recalculate interval for the test so we can be sure that the async workers
	// do run, but then stop running on shutdown
	pollInterval = 2 * time.Second
	recalculateInterval = 100 * time.Millisecond

	// Create the localserver
	ls, err := New(context.TODO(), k)
	require.NoError(t, err)

	// Set the querier
	querier := mocks.NewQuerier(t)
	// On a 2-sec interval, letting the server run for 3 seconds, we should see only one query
	querier.On("Query", mock.Anything).Return(nil, nil).Once()
	ls.SetQuerier(querier)

	// Let the server run for a bit
	go ls.Start()
	time.Sleep(3 * time.Second)
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
			t.Errorf("could not call interrupt multiple times and return within 5 seconds -- received %d interrupts before timeout", receivedInterrupts)
			t.FailNow()
		}
	}

	require.Equal(t, expectedInterrupts, receivedInterrupts)

	k.AssertExpectations(t)
	querier.AssertExpectations(t)
}
