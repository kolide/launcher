package logshipper

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/pkg/agent/flags/keys"
	"github.com/kolide/launcher/pkg/agent/storage"
	storageci "github.com/kolide/launcher/pkg/agent/storage/ci"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestLogShipper(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{
			name: "happy path",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			knapsack := mocks.NewKnapsack(t)
			knapsack.On("RegisterChangeObserver", mock.Anything, keys.LogShippingLevel, keys.LogIngestServerURL)
			knapsack.On("LogShippingLevel").Return("info").Times(5)

			tokenStore := testKVStore(t, storage.TokenStore.String())
			knapsack.On("TokenStore").Return(tokenStore)

			// no auth token
			ls := New(knapsack, log.NewNopLogger())
			require.False(t, ls.isShippingStarted, "shipping should not have stared since there is no auth token")

			// no ingest server url
			authToken := ulid.New()
			tokenStore.Set(storage.ObservabilityIngestAuthTokenKey, []byte(authToken))
			knapsack.On("LogIngestServerURL").Return("").Once()
			ls.Ping()
			require.False(t, ls.isShippingStarted, "shipping should not have stared since there is no ingest server url")
			require.Equal(t, authToken, ls.sender.authtoken)

			// no device identifying attributes
			logIngestUrl := "https://example.com"
			knapsack.On("LogIngestServerURL").Return(logIngestUrl).Times(4)
			knapsack.On("ServerProvidedDataStore").Return(storageci.NewStore(t, log.NewNopLogger(), "test")).Once()
			ls.Ping()
			require.False(t, ls.isShippingStarted, "shipping should not have stared since there are no device identifying attributes")
			require.Equal(t, authToken, ls.sender.authtoken)
			require.Equal(t, logIngestUrl, ls.sender.endpoint)

			// happy path
			knapsack.On("ServerProvidedDataStore").Return(testKVStore(t, storage.ServerProvidedDataStore.String()))
			ls.Ping()
			require.True(t, ls.isShippingStarted, "shipping should now be enabled")
			require.Equal(t, authToken, ls.sender.authtoken)
			require.Equal(t, logIngestUrl, ls.sender.endpoint)

			// update auth token
			authToken = ulid.New()
			tokenStore.Set(storage.ObservabilityIngestAuthTokenKey, []byte(authToken))
			ls.Ping()
			require.Equal(t, authToken, ls.sender.authtoken, "auth token should update")
			require.Equal(t, logIngestUrl, ls.sender.endpoint)

			// update shipping level
			knapsack.On("LogShippingLevel").Return("debug")
			knapsack.On("Slogger").Return(multislogger.New().Logger)
			ls.Ping()
			require.Equal(t, slog.LevelDebug.Level(), ls.slogLevel.Level(), "log shipper should set to debug")
			require.Equal(t, authToken, ls.sender.authtoken)
			require.Equal(t, logIngestUrl, ls.sender.endpoint)

			// update log ingest url
			logIngestUrl = "https://example.com/new"
			knapsack.On("LogIngestServerURL").Return(logIngestUrl)
			ls.Ping()
			require.Equal(t, slog.LevelDebug.Level(), ls.slogLevel.Level(), "log shipper should set to debug")
			require.Equal(t, authToken, ls.sender.authtoken)
			require.Equal(t, logIngestUrl, ls.sender.endpoint)
		})
	}
}

func TestStop_Multiple(t *testing.T) {
	t.Parallel()

	knapsack := mocks.NewKnapsack(t)

	tokenStore := testKVStore(t, storage.TokenStore.String())
	authToken := ulid.New()
	knapsack.On("TokenStore").Return(tokenStore)
	tokenStore.Set(storage.ObservabilityIngestAuthTokenKey, []byte(authToken))

	serverDataStore := testKVStore(t, storage.ServerProvidedDataStore.String())
	knapsack.On("ServerProvidedDataStore").Return(serverDataStore)

	endpoint := "https://someurl"
	knapsack.On("LogIngestServerURL").Return(endpoint).Times(1)
	knapsack.On("ServerProvidedDataStore").Return(tokenStore)
	knapsack.On("LogShippingLevel").Return("debug")
	knapsack.On("Slogger").Return(multislogger.New().Logger)
	knapsack.On("RegisterChangeObserver", mock.Anything, keys.LogShippingLevel, keys.LogIngestServerURL)

	ls := New(knapsack, log.NewNopLogger())

	go ls.Run()
	time.Sleep(3 * time.Second)
	ls.Stop(errors.New("test error"))

	// Confirm we can call Interrupt multiple times without blocking
	interruptComplete := make(chan struct{})
	expectedInterrupts := 3
	for i := 0; i < expectedInterrupts; i += 1 {
		go func() {
			ls.Stop(nil)
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

func TestStopWithoutRun(t *testing.T) {
	t.Parallel()

	knapsack := mocks.NewKnapsack(t)
	tokenStore := testKVStore(t, storage.TokenStore.String())
	authToken := ulid.New()

	knapsack.On("TokenStore").Return(tokenStore)
	tokenStore.Set(storage.ObservabilityIngestAuthTokenKey, []byte(authToken))

	serverDataStore := testKVStore(t, storage.ServerProvidedDataStore.String())
	knapsack.On("ServerProvidedDataStore").Return(serverDataStore)

	endpoint := "https://someurl"
	knapsack.On("LogIngestServerURL").Return(endpoint).Times(1)
	knapsack.On("ServerProvidedDataStore").Return(tokenStore)
	knapsack.On("LogShippingLevel").Return("debug")
	knapsack.On("Slogger").Return(multislogger.New().Logger)
	knapsack.On("RegisterChangeObserver", mock.Anything, keys.LogShippingLevel, keys.LogIngestServerURL)

	ls := New(knapsack, log.NewNopLogger())

	ls.Stop(errors.New("test error"))
}

func testKVStore(t *testing.T, name string) types.KVStore {
	s, err := storageci.NewStore(t, log.NewNopLogger(), name)

	for _, key := range []string{"device_id", "munemo", "organization_id", "serial_number"} {
		s.Set([]byte(key), []byte(ulid.New()))
	}

	require.NoError(t, err)
	return s
}
