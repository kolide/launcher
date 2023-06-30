package logshipper

import (
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/pkg/agent/storage"
	storageci "github.com/kolide/launcher/pkg/agent/storage/ci"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/agent/types/mocks"
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
			tokenStore := testTokenStore(t)
			authToken := ulid.New()

			knapsack.On("TokenStore").Return(tokenStore)
			tokenStore.Set(storage.ObservabilityIngestAuthTokenKey, []byte(authToken))

			endpoint := "https://someurl"
			knapsack.On("LogIngestServerURL").Return(endpoint).Times(2)
			knapsack.On("ServerProvidedDataStore").Return(tokenStore)
			knapsack.On("Debug").Return(true)

			ls := New(knapsack, log.NewNopLogger())

			require.Equal(t, authToken, ls.sender.authtoken)
			require.Equal(t, endpoint, ls.sender.endpoint)
			require.True(t, ls.isShippingEnabled, "shipping should be enabled")

			authToken = ulid.New()
			tokenStore.Set(storage.ObservabilityIngestAuthTokenKey, []byte(authToken))

			endpoint = "http://someotherurl"
			knapsack.On("LogIngestServerURL").Return(endpoint).Times(2)

			ls.Ping()
			require.Equal(t, authToken, ls.sender.authtoken, "log shipper should update auth token on sender")
			require.Equal(t, endpoint, ls.sender.endpoint, "log shipper should update endpoint on sender")
			require.True(t, ls.isShippingEnabled, "shipping should be enabled")

			endpoint = ""
			knapsack.On("LogIngestServerURL").Return(endpoint).Times(3)
			ls.Ping()

			require.Equal(t, authToken, ls.sender.authtoken, "log shipper should update auth token on sender")
			require.Equal(t, endpoint, ls.sender.endpoint, "log shipper should update endpoint on sender")
			require.False(t, ls.isShippingEnabled, "shipping should be disabled due to bad endpoint")
		})
	}
}

func testTokenStore(t *testing.T) types.KVStore {
	s, err := storageci.NewStore(t, log.NewNopLogger(), storage.TokenStore.String())
	require.NoError(t, err)
	return s
}
