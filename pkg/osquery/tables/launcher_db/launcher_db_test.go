package launcher_db

import (
	"path/filepath"
	"testing"

	"github.com/kolide/launcher/pkg/osquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
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

			db := createDb(t, tt.data)
			kvps, err := dbKeyValueRows(osquery.ServerProvidedDataBucket, db)
			require.NoError(t, err)

			assert.ElementsMatch(t, tt.want, kvps)
		})
	}
}

func createDb(t *testing.T, values map[string]string) *bbolt.DB {
	dir := t.TempDir()

	db, err := bbolt.Open(filepath.Join(dir, "db"), 0600, nil)
	require.NoError(t, err)

	err = db.Update(func(tx *bbolt.Tx) error {
		// add the bucket
		bucket, err := tx.CreateBucketIfNotExists([]byte(osquery.ServerProvidedDataBucket))
		require.NoError(t, err)

		// add the values to the bucket
		for key, value := range values {
			require.NoError(t, bucket.Put([]byte(key), []byte(value)))
		}
		return nil
	})

	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	return db
}
