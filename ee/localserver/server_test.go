package localserver

import (
	"errors"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/agent/storage"
	storageci "github.com/kolide/launcher/pkg/agent/storage/ci"
	"github.com/kolide/launcher/pkg/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/stretchr/testify/require"
)

func TestInterrupt_Multiple(t *testing.T) {
	t.Parallel()

	c, err := storageci.NewStore(t, log.NewNopLogger(), storage.ConfigStore.String())
	require.NoError(t, err)
	require.NoError(t, osquery.SetupLauncherKeys(c))

	k := mocks.NewKnapsack(t)
	k.On("KolideServerURL").Return("localserver")
	k.On("ConfigStore").Return(c)
	k.On("Slogger").Return(multislogger.New().Logger)

	ls, err := New(k)
	require.NoError(t, err)

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
}
