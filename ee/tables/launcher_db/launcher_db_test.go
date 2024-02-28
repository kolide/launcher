package launcher_db

import (
	"testing"

	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_generateLauncherDbTable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data map[string]string
		want []map[string]string
	}{
		{
			name: "empty",
			data: map[string]string{},
			want: []map[string]string{},
		},
		{
			name: "single",
			data: map[string]string{"one": "one"},
			want: []map[string]string{
				{
					"key":   "one",
					"value": "one",
				},
			},
		},
		{
			name: "multiple",
			data: map[string]string{
				"one":   "one",
				"two":   "two",
				"three": "three",
			},
			want: []map[string]string{
				{
					"key":   "one",
					"value": "one",
				},
				{
					"key":   "two",
					"value": "two",
				},
				{
					"key":   "three",
					"value": "three",
				},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := setupStorage(t, tt.data)
			kvps, err := dbKeyValueRows(storage.ServerProvidedDataStore.String(), store)
			require.NoError(t, err)

			assert.ElementsMatch(t, tt.want, kvps)
		})
	}
}

func setupStorage(t *testing.T, values map[string]string) types.KVStore {
	s, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.ServerProvidedDataStore.String())
	require.NoError(t, err)

	// add the values to the bucket
	for key, value := range values {
		require.NoError(t, s.Set([]byte(key), []byte(value)))
	}

	return s
}
