package agentbbolt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Updates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		updates []map[string]string
		want    []map[string]string
	}{
		{
			name:    "empty",
			updates: []map[string]string{{}, {}},
			want:    []map[string]string{},
		},
		{
			name:    "single",
			updates: []map[string]string{{"one": "one"}, {"one": "new_one"}},
			want: []map[string]string{
				{
					"key":   "one",
					"value": "new_one",
				},
			},
		},
		{
			name: "multiple",
			updates: []map[string]string{
				{
					"one":   "one",
					"two":   "two",
					"three": "three",
				},
				{
					"one":   "new_one",
					"two":   "new_two",
					"three": "new_three",
				},
			},
			want: []map[string]string{
				{
					"key":   "one",
					"value": "new_one",
				},
				{
					"key":   "two",
					"value": "new_two",
				},
				{
					"key":   "three",
					"value": "new_three",
				},
			},
		},
		{
			name: "delete stale keys",
			updates: []map[string]string{
				{
					"one":   "one",
					"two":   "two",
					"three": "three",
					"four":  "four",
					"five":  "five",
					"six":   "six",
				},
				{
					"four": "four",
				},
			},
			want: []map[string]string{
				{
					"key":   "four",
					"value": "four",
				},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store, err := NewStore(log.NewNopLogger(), SetupDB(t), tt.name)
			require.NoError(t, err)

			for _, update := range tt.updates {
				updateBytes, err := json.Marshal(update)
				require.NoError(t, err)

				store.Update(bytes.NewReader(updateBytes))
			}

			kvps, err := getKeyValueRows(store, tt.name)
			require.NoError(t, err)

			assert.ElementsMatch(t, tt.want, kvps)

			for _, row := range kvps {
				k := row["key"]
				v := row["value"]

				g, err := store.Get([]byte(k))
				assert.NoError(t, err)
				assert.Equal(t, []byte(v), g)
			}
		})
	}
}

func getKeyValueRows(store types.KVStore, bucketName string) ([]map[string]string, error) {
	results := make([]map[string]string, 0)

	if err := store.ForEach(func(k, v []byte) error {
		results = append(results, map[string]string{
			"key":   string(k),
			"value": string(v),
		})
		return nil
	}); err != nil {
		return nil, fmt.Errorf("fetching data: %w", err)
	}

	return results, nil
}
