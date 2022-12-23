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

			db := createDb(t)
			bc := NewBucketConsumer(log.NewNopLogger(), db, tt.name)

			for _, update := range tt.updates {
				updateBytes, err := json.Marshal(update)
				require.NoError(t, err)

				bc.Update(bytes.NewReader(updateBytes))
			}

			kvps, err := getKeyValueRows(db, tt.name)
			require.NoError(t, err)

			assert.ElementsMatch(t, tt.want, kvps)
		})
	}
}

func createDb(t *testing.T) *bbolt.DB {
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
