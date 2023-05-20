package storageci

import (
	"bytes"
	"errors"
	"fmt"
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
	db := SetupDB(t)
	bboltStore, err := agentbbolt.NewStore(logger, db, "test_bucket")
	require.NoError(t, err)

	stores := []types.KVStore{
		inmemory.NewStore(logger),
		bboltStore,
	}
	return stores
}

// Test_GetSetUnusualByteTricks uses bytes, and tries to test some unusual corners of byte and buffer
// operations. This is designed around issues encountered in the memory store
func Test_GetSetUnusualByteTricks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		sets        [][]byte
		gets        [][]byte
		expectedErr bool
	}{

		{
			name: "overwrite",
			sets: [][]byte{[]byte("key"), []byte("short"), []byte("key"), []byte("a long value"), []byte("key"), []byte("s")},
			gets: [][]byte{[]byte("key"), []byte("s")},
		},
		{
			name: "non-stringable",
			sets: [][]byte{
				{1}, {1},
				{2}, {2},
				{1, 2}, {1, 2},
				{0, 0, 0}, {0, 0, 0},
				{1, 0, 0}, {1, 0, 0},
				{2, 0, 0}, {2, 0, 0},
			},
			gets: [][]byte{
				{1}, {1},
				{2}, {2},
				{1, 2}, {1, 2},
				{0, 0, 0}, {0, 0, 0},
				{1, 0, 0}, {1, 0, 0},
				{2, 0, 0}, {2, 0, 0},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			for _, s := range getStores(t) {

				// make sure the test is setup correctly
				require.True(t, len(tt.sets)%2 == 0, "sets should be even")
				require.True(t, len(tt.gets)%2 == 0, "gets should be even")

				buf := bytes.NewBuffer(nil)

				for i := 0; i+1 <= len(tt.sets); i = i + 2 {
					k := tt.sets[i]
					v := tt.sets[i+1]
					buf.Reset()
					buf.Write(v)
					err := s.Set(k, buf.Bytes())
					if tt.expectedErr {
						require.Error(t, err)
						return
					}
					require.NoError(t, err)
				}

				for i := 0; i+1 <= len(tt.gets); i = i + 2 {
					k := tt.gets[i]
					expectedV := tt.gets[i+1]
					actual, err := s.Get(k)
					require.NoError(t, err)
					assert.Equal(t, expectedV, actual)
				}
			}
		})
	}
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

			for _, s := range getStores(t) {
				for _, update := range tt.updates {
					s.Update(update)
				}

				kvps, err := getKeyValueRows(s, tt.name)
				require.NoError(t, err)

				assert.ElementsMatch(t, tt.want, kvps)

				for _, row := range kvps {
					k := row["key"]
					v := row["value"]

					g, err := s.Get([]byte(k))
					assert.NoError(t, err)
					assert.Equal(t, []byte(v), g)
				}
			}
		})
	}
}

func Test_ForEach(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		sets            map[string]string
		gets            map[string]string
		fnFailsOnCall   int
		expectedFnCalls int
	}{
		{
			name: "empty",
			sets: map[string]string{},
			gets: map[string]string{},
		},
		{
			name:            "three calls",
			sets:            map[string]string{"key1": "value1", "key2": "value2", "key3": "value3"},
			gets:            map[string]string{"key1": "value1", "key2": "value2", "key3": "value3"},
			expectedFnCalls: 3,
		},
		{
			name:            "second call fails",
			sets:            map[string]string{"key1": "value1", "key2": "value2", "key3": "value3"},
			gets:            map[string]string{"key1": "value1", "key2": "value2", "key3": "value3"},
			fnFailsOnCall:   2,
			expectedFnCalls: 2,
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
						require.NoError(t, err)
					}()
				}
				wg.Wait()

				var fnCalls int
				fn := func(k, v []byte) error {
					fnCalls = fnCalls + 1
					if tt.fnFailsOnCall > 0 && fnCalls == tt.fnFailsOnCall {
						return errors.New("for each call function failed")
					}
					return nil
				}

				wg.Add(1)
				go func() {
					defer wg.Done()
					err := s.ForEach(fn)
					if tt.fnFailsOnCall > 0 && fnCalls == tt.fnFailsOnCall {
						require.Error(t, err)
					} else {
						require.NoError(t, err)
						assert.Equal(t, tt.expectedFnCalls, fnCalls)
					}
				}()
				wg.Wait()
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
