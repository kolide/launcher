package logshipper

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
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
			knapsack.On("CurrentRunningOsqueryVersion").Return("5.12.3")
			knapsack.On("Slogger").Return(multislogger.NewNopLogger())

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
			knapsack.On("ServerProvidedDataStore").Return(storageci.NewStore(t, multislogger.NewNopLogger(), "test")).Once()
			ls.Ping()
			require.False(t, ls.isShippingStarted, "shipping should not have stared since there are no device identifying attributes")
			require.Equal(t, authToken, ls.sender.authtoken)
			require.Equal(t, logIngestUrl, ls.sender.endpoint)

			// happy path

			// put some stuff in the send buffer
			_, err := ls.sendBuffer.Write([]byte(`{"a":"b"}`))
			require.NoError(t, err)
			_, err = ls.sendBuffer.Write([]byte(`{"c":"d"}`))
			require.NoError(t, err)

			knapsack.On("ServerProvidedDataStore").Return(testKVStore(t, storage.ServerProvidedDataStore.String()))
			ls.Ping()
			require.True(t, ls.isShippingStarted, "shipping should now be enabled")
			require.Equal(t, authToken, ls.sender.authtoken)
			require.Equal(t, logIngestUrl, ls.sender.endpoint)

			// make sure attributes are added to logs in send buffer
			ls.sendBuffer.UpdateData(func(in io.Reader, out io.Writer) error {
				var data map[string]string
				err := json.NewDecoder(in).Decode(&data)
				require.NoError(t, err)

				for k, v := range deviceIdentifyingAttributes {
					require.Equal(t, v, data[k], "device identifying attributes should be in the send buffer")
				}

				// write data back to out
				err = json.NewEncoder(out).Encode(data)
				require.NoError(t, err)
				return nil
			})

			// update auth token
			authToken = ulid.New()
			tokenStore.Set(storage.ObservabilityIngestAuthTokenKey, []byte(authToken))
			ls.Ping()
			require.Equal(t, authToken, ls.sender.authtoken, "auth token should update")
			require.Equal(t, logIngestUrl, ls.sender.endpoint)

			// update shipping level
			knapsack.On("LogShippingLevel").Return("debug")
			knapsack.On("Slogger").Return(multislogger.NewNopLogger())
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
	knapsack.On("CurrentRunningOsqueryVersion").Return("5.12.3")

	endpoint := "https://someurl"
	knapsack.On("LogIngestServerURL").Return(endpoint).Times(1)
	knapsack.On("ServerProvidedDataStore").Return(tokenStore)
	knapsack.On("LogShippingLevel").Return("debug")
	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	knapsack.On("Slogger").Return(slogger)
	knapsack.On("RegisterChangeObserver", mock.Anything, keys.LogShippingLevel, keys.LogIngestServerURL)

	ls := New(knapsack, log.NewNopLogger())

	go ls.Run()
	time.Sleep(3 * time.Second)
	interruptStart := time.Now()
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
			t.Errorf("could not call interrupt multiple times and return within 5 seconds -- interrupted at %s, received %d interrupts before timeout; logs: \n%s\n", interruptStart.String(), receivedInterrupts, logBytes.String())
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
	knapsack.On("Slogger").Return(multislogger.NewNopLogger())
	knapsack.On("RegisterChangeObserver", mock.Anything, keys.LogShippingLevel, keys.LogIngestServerURL)
	knapsack.On("CurrentRunningOsqueryVersion").Return("5.12.3")

	ls := New(knapsack, log.NewNopLogger())

	ls.Stop(errors.New("test error"))
}

var deviceIdentifyingAttributes = map[string]string{
	"device_id":       ulid.New(),
	"munemo":          ulid.New(),
	"organization_id": ulid.New(),
	"serial_number":   ulid.New(),
}

func testKVStore(t *testing.T, name string) types.KVStore {
	s, err := storageci.NewStore(t, multislogger.NewNopLogger(), name)

	for k, v := range deviceIdentifyingAttributes {
		s.Set([]byte(k), []byte(v))
	}

	require.NoError(t, err)
	return s
}

func TestUpdateLogShippingLevel(t *testing.T) {
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
	knapsack.On("Slogger").Return(multislogger.NewNopLogger())
	knapsack.On("RegisterChangeObserver", mock.Anything, keys.LogShippingLevel, keys.LogIngestServerURL)
	knapsack.On("CurrentRunningOsqueryVersion").Return("5.12.3")
	knapsack.On("LogShippingLevel").Return("warn").Once()

	ls := New(knapsack, log.NewNopLogger())
	// new immediately calls Ping -> updateLogShippingLevel, expect that we are initialized with correct log level
	require.Equal(t, slog.LevelWarn, ls.slogLevel.Level())

	knapsack.On("LogShippingLevel").Return("debug").Once()
	ls.updateLogShippingLevel()
	require.Equal(t, slog.LevelDebug, ls.slogLevel.Level())

	knapsack.On("LogShippingLevel").Return("info").Once()
	ls.updateLogShippingLevel()
	require.Equal(t, slog.LevelInfo, ls.slogLevel.Level())

	// we do expect the invalid attempt to be logged, so 2 LogShippingLevel calls are mocked
	knapsack.On("LogShippingLevel").Return("wrongo").Twice()
	ls.updateLogShippingLevel()
	// we don't expect any changes from setting invalid level
	require.Equal(t, slog.LevelInfo, ls.slogLevel.Level())

	// now reattempt with a real value
	knapsack.On("LogShippingLevel").Return("error").Once()
	ls.updateLogShippingLevel()
	require.Equal(t, slog.LevelError, ls.slogLevel.Level())
}
