package storageci

import (
	"sync"
	"testing"

	"github.com/go-kit/kit/log"
	agentbbolt "github.com/kolide/launcher/pkg/agent/storage/bbolt"
	"github.com/kolide/launcher/pkg/agent/storage/inmemory"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getStores(t *testing.T) []types.KVStore {
	logger := log.NewNopLogger()
	db := agentbbolt.SetupDB(t)
	bboltStore, err := agentbbolt.NewStore(logger, db, "test_bucket")
	require.NoError(t, err)

	stores := []types.KVStore{
		inmemory.NewStore(logger),
		bboltStore,
	}
	return stores
}

func Test_GetSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		sets        map[string]string
		gets        map[string]string
		expectedErr bool
	}{
		{
			name: "empty",
			sets: map[string]string{},
			gets: map[string]string{},
		},
		{
			name:        "blank",
			sets:        map[string]string{"": ""},
			gets:        map[string]string{"": ""},
			expectedErr: true,
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

			for _, s := range getStores(t) {
				wg := sync.WaitGroup{}
				for k, v := range tt.sets {
					k, v := k, v
					wg.Add(1)
					go func() {
						defer wg.Done()
						err := s.Set([]byte(k), []byte(v))
						if tt.expectedErr {
							require.Error(t, err)
							return
						}
						require.NoError(t, err)
					}()
				}
				wg.Wait()
				if !tt.expectedErr {
					for k, v := range tt.gets {
						k, v := k, v
						wg.Add(1)
						go func() {
							defer wg.Done()
							val, err := s.Get([]byte(k))
							require.NoError(t, err)
							assert.Equal(t, v, string(val))
						}()
					}
					wg.Wait()
				}
			}
		})
	}
}

func Test_Delete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		sets                map[string]string
		deletes             [][]byte
		expectedRecordCount int
		expectedErr         bool
	}{
		{
			name:    "empty",
			sets:    map[string]string{},
			deletes: [][]byte{},
		},
		{
			name:                "delete nothing",
			sets:                map[string]string{"key1": "value1"},
			deletes:             [][]byte{[]byte("nonexistent-key")},
			expectedRecordCount: 1,
		},
		{
			name:                "delete some",
			sets:                map[string]string{"key1": "value1", "key2": "value2", "key3": "value3", "key4": "value4"},
			deletes:             [][]byte{[]byte("key1"), []byte("key3")},
			expectedRecordCount: 2,
		},
		{
			name:    "delete all",
			sets:    map[string]string{"key1": "value1", "key2": "value2", "key3": "value3"},
			deletes: [][]byte{[]byte("key1"), []byte("key2"), []byte("key3")},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			for _, s := range getStores(t) {
				for k, v := range tt.sets {
					err := s.Set([]byte(k), []byte(v))
					require.NoError(t, err)
				}

				err := s.Delete(tt.deletes...)
				if tt.expectedErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}

				// There should be no records, count and verify
				var recordCount int
				rowFn := func(k, v []byte) error {
					recordCount = recordCount + 1
					return nil
				}
				s.ForEach(rowFn)
				assert.Equal(t, tt.expectedRecordCount, recordCount)
			}
		})
	}
}
