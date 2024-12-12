//go:build darwin
// +build darwin

package secureenclaverunner

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/ee/agent/storage/inmemory"
	"github.com/kolide/launcher/ee/secureenclaverunner/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_secureEnclaveRunner(t *testing.T) {
	t.Parallel()

	privKey, err := echelper.GenerateEcdsaKey()
	require.NoError(t, err)

	t.Run("creates key in public call", func(t *testing.T) {
		t.Parallel()

		secureEnclaveClientMock := mocks.NewSecureEnclaveClient(t)
		secureEnclaveClientMock.On("CreateSecureEnclaveKey", mock.AnythingOfType("string")).Return(&privKey.PublicKey, nil).Once()
		ser, err := New(context.TODO(), multislogger.NewNopLogger(), inmemory.NewStore(), secureEnclaveClientMock)
		require.NoError(t, err)
		require.NotNil(t, ser.Public())

		// key should have been created in public call
		require.Len(t, ser.uidPubKeyMap, 1)
		for _, v := range ser.uidPubKeyMap {
			require.Equal(t, &privKey.PublicKey, v)
		}
	})

	t.Run("creates key in execute", func(t *testing.T) {
		t.Parallel()

		secureEnclaveClientMock := mocks.NewSecureEnclaveClient(t)
		secureEnclaveClientMock.On("CreateSecureEnclaveKey", mock.AnythingOfType("string")).Return(nil, errors.New("not available yet")).Once()
		ser, err := New(context.TODO(), multislogger.NewNopLogger(), inmemory.NewStore(), secureEnclaveClientMock)
		require.NoError(t, err)

		// iniital key should be nil since client wasn't ready
		require.Nil(t, ser.Public())

		// give error on first execute loop
		secureEnclaveClientMock.On("CreateSecureEnclaveKey", mock.AnythingOfType("string")).Return(nil, errors.New("not available yet")).Once()

		// give key on second execute loop
		secureEnclaveClientMock.On("CreateSecureEnclaveKey", mock.AnythingOfType("string")).Return(&privKey.PublicKey, nil).Once()

		go func() {
			// sleep long enough to get through 2 cycles of exectue
			time.Sleep(3 * time.Second)
			ser.Interrupt(errors.New("test"))
		}()

		require.NoError(t, ser.Execute())

		// key should have been created in execute
		require.Len(t, ser.uidPubKeyMap, 1)
		for _, v := range ser.uidPubKeyMap {
			require.Equal(t, &privKey.PublicKey, v)
		}
	})

	t.Run("loads existing key", func(t *testing.T) {
		t.Parallel()

		// populate store with key
		store := inmemory.NewStore()
		firstConsoleUser, err := firstConsoleUser(context.TODO())
		require.NoError(t, err)
		serToSerialize := &secureEnclaveRunner{
			uidPubKeyMap: map[string]*ecdsa.PublicKey{
				firstConsoleUser.Uid: &privKey.PublicKey,
			},
		}
		serJson, err := json.Marshal(serToSerialize)
		require.NoError(t, err)
		err = store.Set([]byte(publicEccDataKey), serJson)
		require.NoError(t, err)

		// create new signer with store containing key
		ser, err := New(context.TODO(), multislogger.NewNopLogger(), store, nil)
		require.NoError(t, err)

		go func() {
			// sleep long enough to get through 2 cycles of exectue
			time.Sleep(3 * time.Second)
			ser.Interrupt(errors.New("test"))
		}()

		require.NoError(t, ser.Execute())

		// key should have been loaded in execute
		require.Len(t, ser.uidPubKeyMap, 1)
		for _, v := range ser.uidPubKeyMap {
			require.Equal(t, &privKey.PublicKey, v)
		}
	})

	t.Run("multiple interrupts", func(t *testing.T) {
		t.Parallel()

		secureEnclaveClientMock := mocks.NewSecureEnclaveClient(t)
		secureEnclaveClientMock.On("CreateSecureEnclaveKey", mock.AnythingOfType("string")).Return(&privKey.PublicKey, nil).Once()

		ser, err := New(context.TODO(), multislogger.NewNopLogger(), inmemory.NewStore(), secureEnclaveClientMock)
		require.NoError(t, err)

		go func() {
			ser.Execute()
		}()

		// Confirm we can call Interrupt multiple times without blocking
		interruptComplete := make(chan struct{})
		expectedInterrupts := 3
		for i := 0; i < expectedInterrupts; i += 1 {
			go func() {
				ser.Interrupt(nil)
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

	t.Run("no console users, creates key", func(t *testing.T) {
		t.Parallel()

		secureEnclaveClientMock := mocks.NewSecureEnclaveClient(t)
		secureEnclaveClientMock.On("CreateSecureEnclaveKey", mock.AnythingOfType("string")).Return(nil, errors.New("not available yet")).Once()
		ser, err := New(context.TODO(), multislogger.NewNopLogger(), inmemory.NewStore(), secureEnclaveClientMock)
		require.NoError(t, err)

		// iniital key should be nil since client wasn't ready
		require.Nil(t, ser.Public())

		// set delay to 100ms for testing
		ser.noConsoleUsersDelay = 100 * time.Millisecond

		// give error on first execute loop
		secureEnclaveClientMock.On("CreateSecureEnclaveKey", mock.AnythingOfType("string")).Return(nil, noConsoleUsersError{}).Once()

		// give key on second execute loop
		secureEnclaveClientMock.On("CreateSecureEnclaveKey", mock.AnythingOfType("string")).Return(&privKey.PublicKey, nil).Once()

		go func() {
			// sleep long enough to get through 2 cycles of exectue
			time.Sleep(3 * time.Second)
			ser.Interrupt(errors.New("test"))
		}()

		require.NoError(t, ser.Execute())

		// key should have been loaded in execute
		require.Len(t, ser.uidPubKeyMap, 1)
		for _, v := range ser.uidPubKeyMap {
			require.Equal(t, &privKey.PublicKey, v)
		}
	})

	t.Run("no console users, handles interrupt", func(t *testing.T) {
		t.Parallel()

		secureEnclaveClientMock := mocks.NewSecureEnclaveClient(t)
		secureEnclaveClientMock.On("CreateSecureEnclaveKey", mock.AnythingOfType("string")).Return(nil, errors.New("not available yet")).Once()
		ser, err := New(context.TODO(), multislogger.NewNopLogger(), inmemory.NewStore(), secureEnclaveClientMock)
		require.NoError(t, err)

		// iniital key should be nil since client wasn't ready
		require.Nil(t, ser.Public())

		// give error on first execute loop
		secureEnclaveClientMock.On("CreateSecureEnclaveKey", mock.AnythingOfType("string")).Return(nil, noConsoleUsersError{}).Once()

		go func() {
			// sleep long enough to get through 2 cycles of exectue
			time.Sleep(3 * time.Second)
			ser.Interrupt(errors.New("test"))
		}()

		require.NoError(t, ser.Execute())

		// no key should be created since loop didn't execute
		// and public not called
		require.Len(t, ser.uidPubKeyMap, 0)
	})
}
