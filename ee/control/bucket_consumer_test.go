package control

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

func Test_Update(t *testing.T) {
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

			db := createDb(t, tt.data)
			bc := NewBucketConsumer(log.NewNopLogger(), db, tt.name)

			dataBytes, err := json.Marshal(tt.data)
			require.NoError(t, err)

			bc.Update(bytes.NewReader(dataBytes))

			kvps, err := getKeyValueRows(db, tt.name)
			require.NoError(t, err)

			assert.ElementsMatch(t, tt.want, kvps)
		})
	}
}

func createDb(t *testing.T, values map[string]string) *bbolt.DB {
	dir := t.TempDir()

	db, err := bbolt.Open(filepath.Join(dir, "db"), 0600, nil)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	return db
}

func getKeyValueRows(db *bbolt.DB, bucketName string) ([]map[string]string, error) {
	results := make([]map[string]string, 0)

	if err := db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b == nil {
			return fmt.Errorf("%s bucket not found", bucketName)
		}

		b.ForEach(func(k, v []byte) error {
			results = append(results, map[string]string{
				"key":   string(k),
				"value": string(v),
			})
			return nil
		})
		return nil
	}); err != nil {
		return nil, fmt.Errorf("fetching data: %w", err)
	}

	return results, nil
}
