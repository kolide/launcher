package storage

import (
	"fmt"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

func Test_GetSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		sets map[string]string
		gets map[string]string
	}{
		{
			name: "empty",
			sets: map[string]string{},
			gets: map[string]string{},
		},
		{
			name: "multiple",
			sets: map[string]string{"key1": "value1", "key2": "value2", "key3": "value3"},
			gets: map[string]string{"key1": "value1", "key2": "value2", "key3": "value3"},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db := setupDB(t)
			bc := NewBBoltKeyValueStore(log.NewNopLogger(), db, tt.name)

			for k, v := range tt.sets {
				err := bc.Set([]byte(k), []byte(v))
				require.NoError(t, err)
			}

			for k, v := range tt.gets {
				val, err := bc.Get([]byte(k))
				require.NoError(t, err)
				assert.Equal(t, v, string(val))
			}
		})
	}
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
