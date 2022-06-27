package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

func TestNewInstance(t *testing.T) { // nolint:paralleltest
	tests := []struct {
		name             string
		initialInstances []*Instance
		wantNumInstances int
		wantErr          error
	}{
		{
			name:             "zero_value",
			wantNumInstances: 1,
		},
		{
			name: "existing_instances",
			initialInstances: []*Instance{
				{
					StartTime: "first_start_time",
					ExitTime:  "first_exit_time",
				},
				{
					StartTime: "second_start_time",
					ExitTime:  "second_exit_time",
				},
			},
			wantNumInstances: 3,
		},
		{
			name: "max_instances_reached",
			initialInstances: []*Instance{
				{}, {}, {}, {}, {}, {}, {}, {}, {},
				{
					ExitTime: "last_exit_time",
				},
			},
			wantNumInstances: 10,
		},
	}
	for _, tt := range tests { // nolint:paralleltest
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() { currentHistory = &History{} })

			require.NoError(t, InitHistory(newTestBoltDb(t, tt.initialInstances...)))

			_, err := NewInstance()

			assert.Equal(t, tt.wantNumInstances, len(currentHistory.instances), "expect history length to reflect new instance")

			assert.ErrorIs(t, err, tt.wantErr)
			if tt.wantErr != nil {
				return
			}

			currInstance, err := LatestInstance()
			assert.NoError(t, err, "expect no error getting current instance")

			// make sure start time was set
			_, err = time.Parse(time.RFC3339, currInstance.StartTime)
			assert.NoError(t, err, "expect start time to be set")

			// verify host name
			hostname, err := os.Hostname()
			assert.NoError(t, err, "expect on error getting host name")

			assert.Equal(t, hostname, currInstance.Hostname, "expect hostname to be set on instance")
		})
	}
}

func TestGetHistory(t *testing.T) { // nolint:paralleltest
	tests := []struct {
		name             string
		initialInstances []*Instance
		want             []Instance
		errString        string
	}{
		{
			name: "success",
			initialInstances: []*Instance{
				{
					StartTime: "first_expected_start_time",
				},
				{
					StartTime: "second_expected_start_time",
				},
			},
			want: []Instance{
				{
					StartTime: "first_expected_start_time",
				},
				{
					StartTime: "second_expected_start_time",
				},
			},
		},
		{
			name:      "no_history_error",
			errString: NoInstancesError{}.Error(),
		},
	}
	for _, tt := range tests { // nolint:paralleltest
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() { currentHistory = &History{} })

			require.NoError(t, InitHistory(newTestBoltDb(t, tt.initialInstances...)))

			got, err := GetHistory()

			if tt.errString != "" {
				assert.EqualError(t, err, tt.errString)
			}

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLatestInstance(t *testing.T) { // nolint:paralleltest
	tests := []struct {
		name             string
		initialInstances []*Instance
		want             Instance
		errString        string
	}{
		{
			name: "success",
			initialInstances: []*Instance{
				{
					StartTime: "first_expected_start_time",
				},
				{
					StartTime: "second_expected_start_time",
				},
			},
			want: Instance{
				StartTime: "second_expected_start_time",
			},
		},
		{
			name:      "no_instances_error",
			errString: NoInstancesError{}.Error(),
		},
	}
	for _, tt := range tests { // nolint:paralleltest
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() { currentHistory = &History{} })

			require.NoError(t, InitHistory(newTestBoltDb(t, tt.initialInstances...)))

			got, err := LatestInstance()

			if tt.errString != "" {
				assert.EqualError(t, err, tt.errString)
			}

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNoDbError(t *testing.T) { // nolint:paralleltest
	err := InitHistory(nil)
	assert.ErrorIs(t, err, NoDbError{})

	err = currentHistory.load()
	assert.ErrorIs(t, err, NoDbError{})

	err = currentHistory.save()
	assert.ErrorIs(t, err, NoDbError{})
}

// newTestBoltDb creates a new boltdb instance and seeds it with the given instances.
func newTestBoltDb(t *testing.T, seedInstances ...*Instance) *bbolt.DB {
	dir := t.TempDir()

	db, err := bbolt.Open(filepath.Join(dir, "osquery_instance_history_test.db"), 0600, &bbolt.Options{
		Timeout: 1 * time.Second,
	})

	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	require.NoError(t, err, "expect no error opening bolt db")

	json, err := json.Marshal(seedInstances)
	require.NoError(t, err, "expect no error marshalling instances")

	err = db.Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte("osquery_instance_history"))
		require.NoError(t, err, "expect no error creating bucket")

		err = bucket.Put([]byte(osqueryHistoryInstanceKey), json)
		require.NoError(t, err, "expect no error writing history to bucket")

		return nil
	})

	return db
}
