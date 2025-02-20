//go:build darwin
// +build darwin

package secureenclaverunner

import (
	"context"
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
		secureEnclaveClientMock.On("CreateSecureEnclaveKey", mock.Anything, mock.AnythingOfType("string")).Return(&privKey.PublicKey, nil).Once()
		ser, err := New(context.TODO(), multislogger.NewNopLogger(), inmemory.NewStore(), secureEnclaveClientMock)
		require.NoError(t, err)
		require.NotNil(t, ser.Public())

		// key should have been created in public call
		require.Len(t, ser.uidPubKeyMap, 1)
		for _, v := range ser.uidPubKeyMap {
			require.Equal(t, &privKey.PublicKey, v.pubKey)
			require.Equal(t, true, v.verifiedInSecureEnclave,
				"key should have been verified in secure enclave since just created",
			)
		}

		// calling public here to make sure we don't try to verify key again
		require.NotNil(t, ser.Public())
	})

	t.Run("creates key in execute", func(t *testing.T) {
		t.Parallel()

		secureEnclaveClientMock := mocks.NewSecureEnclaveClient(t)
		secureEnclaveClientMock.On("CreateSecureEnclaveKey", mock.Anything, mock.AnythingOfType("string")).Return(nil, errors.New("not available yet")).Once()
		ser, err := New(context.TODO(), multislogger.NewNopLogger(), inmemory.NewStore(), secureEnclaveClientMock)
		require.NoError(t, err)

		// iniital key should be nil since client wasn't ready
		require.Nil(t, ser.Public())

		// give error on first execute loop
		secureEnclaveClientMock.On("CreateSecureEnclaveKey", mock.Anything, mock.AnythingOfType("string")).Return(nil, errors.New("not available yet")).Once()

		// give key on second execute loop
		secureEnclaveClientMock.On("CreateSecureEnclaveKey", mock.Anything, mock.AnythingOfType("string")).Return(&privKey.PublicKey, nil).Once()

		go func() {
			// sleep long enough to get through 2 cycles of exectue
			time.Sleep(3 * time.Second)
			ser.Interrupt(errors.New("test"))
		}()

		require.NoError(t, ser.Execute())

		// calling public here to make sure we don't try to verify key again
		require.NotNil(t, ser.Public())

		// key should have been created in execute
		require.Len(t, ser.uidPubKeyMap, 1)
		for _, v := range ser.uidPubKeyMap {
			require.Equal(t, &privKey.PublicKey, v.pubKey)
			require.Equal(t, true, v.verifiedInSecureEnclave,
				"key should have been verified in secure enclave since just created",
			)
		}
	})

	t.Run("loads existing and verifies existing key", func(t *testing.T) {
		t.Parallel()

		// populate store with key
		store := inmemory.NewStore()
		firstConsoleUser, err := firstConsoleUser(context.TODO())
		require.NoError(t, err)

		serToSerialize := &secureEnclaveRunner{
			uidPubKeyMap: map[string]*keyEntry{
				firstConsoleUser.Uid: {
					pubKey: &privKey.PublicKey,
					// setting this to true just to make sure it does NOT get serialized
					// should always start a new run as false
					verifiedInSecureEnclave: true,
				},
			},
		}
		serJson, err := json.Marshal(serToSerialize)
		require.NoError(t, err)
		err = store.Set([]byte(publicEccDataKey), serJson)
		require.NoError(t, err)

		secureEnclaveClientMock := mocks.NewSecureEnclaveClient(t)
		secureEnclaveClientMock.On("VerifySecureEnclaveKey", mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Once()

		// create new signer with store containing key
		ser, err := New(context.TODO(), multislogger.NewNopLogger(), store, secureEnclaveClientMock)
		require.NoError(t, err)

		go func() {
			// sleep long enough to get through 2 cycles of exectue
			time.Sleep(3 * time.Second)
			ser.Interrupt(errors.New("test"))
		}()

		require.NoError(t, ser.Execute())

		// calling public here to make sure we don't try to verify key again
		require.NotNil(t, ser.Public())

		// key should have been loaded in execute
		require.Len(t, ser.uidPubKeyMap, 1)
		for _, v := range ser.uidPubKeyMap {
			require.Equal(t, &privKey.PublicKey, v.pubKey)
			require.Equal(t, true, v.verifiedInSecureEnclave,
				"key should have been verified in secure enclave",
			)
		}
	})

	t.Run("multiple interrupts", func(t *testing.T) {
		t.Parallel()

		secureEnclaveClientMock := mocks.NewSecureEnclaveClient(t)
		secureEnclaveClientMock.On("CreateSecureEnclaveKey", mock.Anything, mock.AnythingOfType("string")).Return(&privKey.PublicKey, nil).Once()

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

	t.Run("no console users then creates key", func(t *testing.T) {
		t.Parallel()

		secureEnclaveClientMock := mocks.NewSecureEnclaveClient(t)
		secureEnclaveClientMock.On("CreateSecureEnclaveKey", mock.Anything, mock.AnythingOfType("string")).Return(nil, errors.New("not available yet")).Once()
		ser, err := New(context.TODO(), multislogger.NewNopLogger(), inmemory.NewStore(), secureEnclaveClientMock)
		require.NoError(t, err)

		// iniital key should be nil since client wasn't ready
		require.Nil(t, ser.Public())

		// set delay to 100ms for testing
		ser.noConsoleUsersDelay = 100 * time.Millisecond

		// give error on first execute loop
		secureEnclaveClientMock.On("CreateSecureEnclaveKey", mock.Anything, mock.AnythingOfType("string")).Return(nil, noConsoleUsersError{}).Once()

		// give error on first execute loop
		secureEnclaveClientMock.On("CreateSecureEnclaveKey", mock.Anything, mock.AnythingOfType("string")).Return(nil, errors.New("some other error")).Once()

		// give key on second execute loop
		secureEnclaveClientMock.On("CreateSecureEnclaveKey", mock.Anything, mock.AnythingOfType("string")).Return(&privKey.PublicKey, nil).Once()

		go func() {
			// sleep long enough to get through 2 cycles of exectue
			time.Sleep(3 * time.Second)
			ser.Interrupt(errors.New("test"))
		}()

		require.NoError(t, ser.Execute())

		// calling public here to make sure we don't try to verify key again
		require.NotNil(t, ser.Public())

		// key should have been loaded in execute
		require.Len(t, ser.uidPubKeyMap, 1)
		for _, v := range ser.uidPubKeyMap {
			require.Equal(t, &privKey.PublicKey, v.pubKey)
			require.Equal(t, true, v.verifiedInSecureEnclave,
				"key should have been verified in secure enclave since just created",
			)
		}
	})

	t.Run("no console users, handles interrupt", func(t *testing.T) {
		t.Parallel()

		secureEnclaveClientMock := mocks.NewSecureEnclaveClient(t)
		secureEnclaveClientMock.On("CreateSecureEnclaveKey", mock.Anything, mock.AnythingOfType("string")).Return(nil, errors.New("not available yet")).Once()
		ser, err := New(context.TODO(), multislogger.NewNopLogger(), inmemory.NewStore(), secureEnclaveClientMock)
		require.NoError(t, err)

		// iniital key should be nil since client wasn't ready
		require.Nil(t, ser.Public())

		// give error on first execute loop
		secureEnclaveClientMock.On("CreateSecureEnclaveKey", mock.Anything, mock.AnythingOfType("string")).Return(nil, noConsoleUsersError{}).Once()

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

	t.Run("creates new key when existing not found in secure enclave", func(t *testing.T) {
		t.Parallel()

		// populate store with key
		store := inmemory.NewStore()
		firstConsoleUser, err := firstConsoleUser(context.TODO())
		require.NoError(t, err)

		serToSerialize := &secureEnclaveRunner{
			uidPubKeyMap: map[string]*keyEntry{
				firstConsoleUser.Uid: {
					pubKey: &privKey.PublicKey,
					// setting this to true just to make sure it does NOT get serialized
					// should always start a new run as false
					verifiedInSecureEnclave: true,
				},
			},
		}
		serJson, err := json.Marshal(serToSerialize)
		require.NoError(t, err)
		err = store.Set([]byte(publicEccDataKey), serJson)
		require.NoError(t, err)

		newKey, err := echelper.GenerateEcdsaKey()
		require.NoError(t, err)

		secureEnclaveClientMock := mocks.NewSecureEnclaveClient(t)

		// report key doesnt exist
		secureEnclaveClientMock.On("VerifySecureEnclaveKey", mock.Anything, mock.Anything, mock.Anything).Return(false, nil).Once()

		// create new key
		secureEnclaveClientMock.On("CreateSecureEnclaveKey", mock.Anything, mock.AnythingOfType("string")).Return(&newKey.PublicKey, nil).Once()

		// create new signer with store containing key
		ser, err := New(context.TODO(), multislogger.NewNopLogger(), store, secureEnclaveClientMock)
		require.NoError(t, err)

		go func() {
			// sleep long enough to get through 2 cycles of exectue
			time.Sleep(3 * time.Second)
			ser.Interrupt(errors.New("test"))
		}()

		require.NoError(t, ser.Execute())

		// calling public here to make sure we don't try to verify key again
		require.NotNil(t, ser.Public())

		// old key should have been replaced with new one
		require.Len(t, ser.uidPubKeyMap, 1)
		for _, v := range ser.uidPubKeyMap {
			require.Equal(t, &newKey.PublicKey, v.pubKey,
				"key should have been replaced with new one",
			)
			require.Equal(t, true, v.verifiedInSecureEnclave,
				"key should have been verified in secure enclave since just created",
			)
		}
	})

	t.Run("does not delete key when cant verify if in secure enclave", func(t *testing.T) {
		t.Parallel()

		// populate store with key
		store := inmemory.NewStore()
		firstConsoleUser, err := firstConsoleUser(context.TODO())
		require.NoError(t, err)

		serToSerialize := &secureEnclaveRunner{
			uidPubKeyMap: map[string]*keyEntry{
				firstConsoleUser.Uid: {
					pubKey: &privKey.PublicKey,
					// setting this to true just to make sure it does NOT get serialized
					// should always start a new run as false
					verifiedInSecureEnclave: true,
				},
			},
		}
		serJson, err := json.Marshal(serToSerialize)
		require.NoError(t, err)
		err = store.Set([]byte(publicEccDataKey), serJson)
		require.NoError(t, err)

		secureEnclaveClientMock := mocks.NewSecureEnclaveClient(t)

		// report error verifying key
		secureEnclaveClientMock.On("VerifySecureEnclaveKey", mock.Anything, mock.Anything, mock.Anything).Return(false, errors.New("cant talk to secure enclave"))

		// create new signer with store containing key
		ser, err := New(context.TODO(), multislogger.NewNopLogger(), store, secureEnclaveClientMock)
		require.NoError(t, err)

		go func() {
			// sleep long enough to get through 2 cycles of exectue
			time.Sleep(3 * time.Second)
			ser.Interrupt(errors.New("test"))
		}()

		require.NoError(t, ser.Execute())

		require.Len(t, ser.uidPubKeyMap, 1)
		for _, v := range ser.uidPubKeyMap {
			require.Equal(t, &privKey.PublicKey, v.pubKey,
				"key should not have been replaced with new one",
			)
			require.Equal(t, false, v.verifiedInSecureEnclave,
				"key should not have been verified in secure enclave",
			)
		}
	})
}
