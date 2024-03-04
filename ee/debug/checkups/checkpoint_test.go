package checkups

import (
	"errors"
	"testing"
	"time"

	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	"github.com/kolide/launcher/ee/agent/types"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func TestInterrupt_Multiple(t *testing.T) {
	t.Parallel()

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("UpdateChannel").Return("nightly").Maybe()
	mockKnapsack.On("TufServerURL").Return("localhost").Maybe()
	mockKnapsack.On("BboltDB").Return(storageci.SetupDB(t)).Maybe()
	mockKnapsack.On("KolideHosted").Return(false).Maybe()
	mockKnapsack.On("KolideServerURL").Return("localhost").Maybe()
	mockKnapsack.On("ControlServerURL").Return("localhost").Maybe()
	mockKnapsack.On("TraceIngestServerURL").Return("localhost").Maybe()
	mockKnapsack.On("LogIngestServerURL").Return("localhost").Maybe()
	mockKnapsack.On("InsecureTransportTLS").Return(true).Maybe()
	mockKnapsack.On("InModernStandby").Return(false).Maybe()
	mockKnapsack.On("RootDirectory").Return("").Maybe()
	mockKnapsack.On("Autoupdate").Return(true).Maybe()
	mockKnapsack.On("LatestOsquerydPath").Return("").Maybe()
	mockKnapsack.On("ServerProvidedDataStore").Return(nil).Maybe()
	mockKnapsack.On("CurrentEnrollmentStatus").Return(types.Enrolled, nil).Maybe()
	checkupLogger := NewCheckupLogger(multislogger.NewNopLogger(), mockKnapsack)
	mockKnapsack.AssertExpectations(t)

	// Start and then interrupt
	go checkupLogger.Run()
	checkupLogger.Interrupt(errors.New("test error"))

	// Confirm we can call Interrupt multiple times without blocking
	interruptComplete := make(chan struct{})
	expectedInterrupts := 3
	for i := 0; i < expectedInterrupts; i += 1 {
		go func() {
			checkupLogger.Interrupt(nil)
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
