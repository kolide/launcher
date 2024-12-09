package tpmrunner

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/ee/agent/storage/inmemory"
	"github.com/kolide/launcher/ee/tpmrunner/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
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

		// public will be nil until execute is called and key is loaded form tpm
		require.Nil(t, tpmRunner.Public())

		tpmSignerCreatorMock.On("New", fakePrivData, fakePubData).Return(privKey, nil).Once()

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
		tpmRunner, err := New(context.TODO(), multislogger.NewNopLogger(), inmemory.NewStore(), withTpmSignerCreator(tpmSignerCreatorMock))
		require.NoError(t, err)

		require.Nil(t, tpmRunner.Public())

		tpmSignerCreatorMock.On("CreateKey").Return(fakePrivData, fakePubData, nil).Once()
		tpmSignerCreatorMock.On("New", fakePrivData, fakePubData).Return(privKey, nil).Once()

		go func() {
			tpmRunner.Execute()
		}()

		// Confirm we can call Interrupt multiple times without blocking
		interruptComplete := make(chan struct{})
		expectedInterrupts := 3
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
				t.Errorf("could not call interrupt multiple times and return within 5 seconds -- received %d interrupts before timeout", receivedInterrupts)
				t.FailNow()
			}
		}

		require.Equal(t, expectedInterrupts, receivedInterrupts)
	})
}
