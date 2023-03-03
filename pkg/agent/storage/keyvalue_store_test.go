package storage

import (
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getStores(t *testing.T) []types.KVStore {
	logger := log.NewNopLogger()
	db := setupDB(t)
	bboltStore, err := NewBBoltKeyValueStore(logger, db, "test_bucket")
	require.NoError(t, err)

	stores := []types.KVStore{
		NewInMemoryKeyValueStore(logger),
		bboltStore,
	}
	return stores
}

func Test_GetSet(t *testing.T) {
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
			for _, s := range getStores(t) {
				for k, v := range tt.sets {
					err := s.Set([]byte(k), []byte(v))
					if tt.expectedErr {
						require.Error(t, err)
					} else {
						require.NoError(t, err)
					}
				}

				if !tt.expectedErr {
					for k, v := range tt.gets {
						val, err := s.Get([]byte(k))
						require.NoError(t, err)
						assert.Equal(t, v, string(val))
					}
				}
			}
		})
	}
}

func Test_Delete(t *testing.T) {
	tests := []struct {
		name                string
		sets                map[string]string
		deletes             []string
		expectedRecordCount int
		expectedErr         bool
	}{
		{
			name:    "empty",
			sets:    map[string]string{},
			deletes: []string{},
		},
		{
			name:                "delete nothing",
			sets:                map[string]string{"key1": "value1"},
			deletes:             []string{"nonexistent-key"},
			expectedRecordCount: 1,
		},
		{
			name:                "delete some",
			sets:                map[string]string{"key1": "value1", "key2": "value2", "key3": "value3", "key4": "value4"},
			deletes:             []string{"key1", "key3"},
			expectedRecordCount: 2,
		},
		{
			name:    "delete all",
			sets:    map[string]string{"key1": "value1", "key2": "value2", "key3": "value3"},
			deletes: []string{"key1", "key2", "key3"},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			for _, s := range getStores(t) {
				for k, v := range tt.sets {
					err := s.Set([]byte(k), []byte(v))
					require.NoError(t, err)
				}

				for _, k := range tt.deletes {
					err := s.Delete([]byte(k))
					if tt.expectedErr {
						require.Error(t, err)
					} else {
						require.NoError(t, err)
					}
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
