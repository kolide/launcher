//go:build !darwin
// +build !darwin

package tpmrunner

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/ee/agent/storage/inmemory"
	"github.com/kolide/launcher/ee/tpmrunner/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
)

func withTpmSignerCreator(tpmSignerCreator tpmSignerCreator) tpmRunnerOption {
	return func(t *tpmRunner) {
		t.signerCreator = tpmSignerCreator
	}
}

func Test_tpmRunner(t *testing.T) {
	t.Parallel()

	privKey, err := echelper.GenerateEcdsaKey()
	require.NoError(t, err)

	fakePrivData, fakePubData := []byte("fake priv data"), []byte("fake pub data")

	t.Run("creates key in execute", func(t *testing.T) {
		t.Parallel()

		tpmSignerCreatorMock := mocks.NewTpmSignerCreator(t)
		tpmRunner, err := New(context.TODO(), multislogger.NewNopLogger(), inmemory.NewStore(), withTpmSignerCreator(tpmSignerCreatorMock))
		require.NoError(t, err)

		// force the runner to think the machine has a TPM
		// not using usual detection methods since were
		// mocking it
		tpmRunner.machineHasTpm.Store(true)

		tpmSignerCreatorMock.On("CreateKey").Return(nil, nil, errors.New("not available yet")).Once()
		require.Nil(t, tpmRunner.Public())

		tpmSignerCreatorMock.On("CreateKey").Return(nil, nil, errors.New("not available yet")).Once()
		tpmSignerCreatorMock.On("CreateKey").Return(fakePrivData, fakePubData, nil).Once()
		tpmSignerCreatorMock.On("New", fakePrivData, fakePubData).Return(privKey, nil).Once()

		go func() {
			// sleep long enough to get through 2 cycles of execute
			time.Sleep(3 * time.Second)
			tpmRunner.Interrupt(errors.New("test"))
		}()

		require.NoError(t, tpmRunner.Execute())
		require.NotNil(t, tpmRunner.Public())
	})

	t.Run("loads existing key", func(t *testing.T) {
		t.Parallel()

		// populate store with key info
		store := inmemory.NewStore()
		store.Set([]byte(privateEccData), fakePrivData)
		store.Set([]byte(publicEccData), fakePubData)

		tpmSignerCreatorMock := mocks.NewTpmSignerCreator(t)
		tpmRunner, err := New(context.TODO(), multislogger.NewNopLogger(), store, withTpmSignerCreator(tpmSignerCreatorMock))
		require.NoError(t, err)

		// force the runner to think the machine has a TPM
		// not using usual detection methods since were
		// mocking it
		tpmRunner.machineHasTpm.Store(true)

		tpmSignerCreatorMock.On("New", fakePrivData, fakePubData).Return(privKey, nil).Once()

		// the call to public should load the key from the store and signer creator should not be called any more after
		require.NotNil(t, tpmRunner.Public())

		go func() {
			// sleep long enough to get through 2 cycles of exectue
			time.Sleep(3 * time.Second)
			tpmRunner.Interrupt(errors.New("test"))
		}()

		require.NoError(t, tpmRunner.Execute())
		require.NotNil(t, tpmRunner.Public())
	})

	t.Run("test multiple interrupts", func(t *testing.T) {
		t.Parallel()

		tpmSignerCreatorMock := mocks.NewTpmSignerCreator(t)
		var logBytes threadsafebuffer.ThreadSafeBuffer
		slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
		tpmRunner, err := New(context.TODO(), slogger, inmemory.NewStore(), withTpmSignerCreator(tpmSignerCreatorMock))
		require.NoError(t, err)

		// force the runner to think the machine has a TPM
		// not using usual detection methods since were
		// mocking it
		tpmRunner.machineHasTpm.Store(true)

		tpmSignerCreatorMock.On("CreateKey").Return(nil, nil, errors.New("not available yet")).Once()
		require.Nil(t, tpmRunner.Public())

		tpmSignerCreatorMock.On("CreateKey").Return(fakePrivData, fakePubData, nil).Once()
		tpmSignerCreatorMock.On("New", fakePrivData, fakePubData).Return(privKey, nil).Once()

		go func() {
			tpmRunner.Execute()
		}()
		time.Sleep(3 * time.Second)

		// Confirm we can call Interrupt multiple times without blocking
		interruptComplete := make(chan struct{})
		expectedInterrupts := 3
		interruptStart := time.Now()
		for i := 0; i < expectedInterrupts; i += 1 {
			go func() {
				tpmRunner.Interrupt(nil)
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
	})
}
