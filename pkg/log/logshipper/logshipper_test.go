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
			tokenStore.Set(observabilityIngestTokenKey, []byte(authToken))

			knapsack.On("DisableObservabilityIngestTLS").Return(true)
			knapsack.On("ObservabilityIngestServerURL").Return("http://someurl").Once()
			knapsack.On("Debug").Return(true)

			ls, err := New(knapsack)
			require.NoError(t, err)

			require.Equal(t, authToken, ls.sender.authtoken)

			authToken = ulid.New()
			tokenStore.Set(observabilityIngestTokenKey, []byte(authToken))

			newEndpoint := "http://someotherurl"
			knapsack.On("ObservabilityIngestServerURL").Return(newEndpoint)

			ls.Ping()
			require.Equal(t, authToken, ls.sender.authtoken, "log shipper should update auth token on sender")
			require.Equal(t, newEndpoint, ls.sender.endpoint, "log shipper should update endpoint on sender")
		})
	}
}

func testTokenStore(t *testing.T) types.KVStore {
	s, err := storageci.NewStore(t, log.NewNopLogger(), storage.TokenStore.String())
	require.NoError(t, err)
	return s
}
