package history

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewInstance(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		// registrationId will automatically be added to any new instances (if missing in test case) before seeding
		registrationId   string
		initialInstances []*Instance
		wantNumInstances int
		wantErr          error
	}{
		{
			name:             "zero_value",
			wantNumInstances: 1,
			registrationId:   ulid.New(),
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
			registrationId:   ulid.New(),
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
			registrationId:   ulid.New(),
		},
	}
	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// make sure registration ids are set to ensure proper behavior when searching by registration id later
			for idx, instance := range tt.initialInstances {
				if instance.RegistrationId == "" {
					tt.initialInstances[idx].RegistrationId = tt.registrationId
				}
			}

			currentHistory, err := InitHistory(setupStorage(t, tt.initialInstances...))
			require.NoError(t, err, "expected to be able to initialize history without error")

			err = currentHistory.NewInstance(tt.registrationId, ulid.New())
			require.NoError(t, err, "expected to be able to add new instance to history")

			assert.Equal(t, tt.wantNumInstances, len(currentHistory.instances), "expect history length to reflect new instance")

			assert.ErrorIs(t, err, tt.wantErr)
			if tt.wantErr != nil {
				return
			}

			currInstance, err := currentHistory.latestInstance(tt.registrationId)
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

func TestGetHistory(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		initialInstances []*Instance
		want             []map[string]string
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
			want: []map[string]string{
				{"connect_time": "", "errors": "", "exit_time": "", "hostname": "", "instance_id": "", "instance_run_id": "", "registration_id": "", "start_time": "first_expected_start_time", "version": ""},
				{"connect_time": "", "errors": "", "exit_time": "", "hostname": "", "instance_id": "", "instance_run_id": "", "registration_id": "", "start_time": "second_expected_start_time", "version": ""},
			},
		},
		{
			name:      "no_history_error",
			errString: NoInstancesError{}.Error(),
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			currentHistory, err := InitHistory(setupStorage(t, tt.initialInstances...))
			require.NoError(t, err, "expected to be able to initialize history without error")

			got, err := currentHistory.GetHistory()

			if tt.errString != "" {
				assert.EqualError(t, err, tt.errString)
			}

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLatestInstance(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		initialInstances []*Instance
		want             *Instance
		errString        string
	}{
		{
			name: "success",
			initialInstances: []*Instance{
				{
					StartTime: "first_expected_start_time",
					RegistrationId: types.DefaultRegistrationID,
				},
				{
					StartTime: "second_expected_start_time",
					RegistrationId: types.DefaultRegistrationID,
				},
			},
			want: &Instance{
				StartTime: "second_expected_start_time",
				RegistrationId: types.DefaultRegistrationID,
			},
		},
		{
			name:      "no_instances_error",
			errString: NoInstancesError{}.Error(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			currentHistory, err := InitHistory(setupStorage(t, tt.initialInstances...))
			require.NoError(t, err, "expected to be able to initialize history without error")

			got, err := currentHistory.latestInstance(types.DefaultRegistrationID)

			if tt.errString != "" {
				assert.EqualError(t, err, tt.errString)
			}

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLatestInstanceStats(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		registrationID   string
		initialInstances []*Instance
		want             map[string]string
		errString        string
	}{
		{
			name:           "success",
			registrationID: "test",
			initialInstances: []*Instance{
				{
					StartTime:      "first_expected_start_time",
					RegistrationId: "some other",
				},
				{
					StartTime:      "second_expected_start_time",
					RegistrationId: "test",
				},
				{
					StartTime:      "third_expected_start_time",
					RegistrationId: "test",
				},
				{
					StartTime:      "fourth_expected_start_time",
					RegistrationId: "another",
				},
			},
			want: map[string]string{
				"connect_time": "",
				"errors": "",
				"exit_time": "",
				"hostname": "",
				"instance_id": "",
				"instance_run_id": "",
				"registration_id": "test",
				"start_time": "third_expected_start_time",
				"version": "",
			},
		},
		{
			name:           "no_instances_error",
			registrationID: "test",
			errString:      NoInstancesError{}.Error(),
		},
		{
			name:           "no matching instances",
			registrationID: "test3",
			initialInstances: []*Instance{
				{
					StartTime:      "first_expected_start_time",
					RegistrationId: "test",
				},
				{
					StartTime:      "second_expected_start_time",
					RegistrationId: "test1",
				},
				{
					StartTime:      "third_expected_start_time",
					RegistrationId: "test2",
				},
			},
			errString: NoInstancesError{}.Error(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			currentHistory, err := InitHistory(setupStorage(t, tt.initialInstances...))
			require.NoError(t, err, "expected to be able to initialize history without error")

			got, err := currentHistory.LatestInstanceStats(tt.registrationID)

			if tt.errString != "" {
				assert.EqualError(t, err, tt.errString)
			}

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLatestInstanceUptimeMinutes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		initialInstances []*Instance
		want             int64
		expectedErr      bool
	}{
		{
			name: "success",
			initialInstances: []*Instance{
				{
					RegistrationId: types.DefaultRegistrationID,
					StartTime: time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339),
				},
				{
					RegistrationId: types.DefaultRegistrationID,
					StartTime: time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339),
				},
			},
			want:        10,
			expectedErr: false,
		},
		{
			name:        "no_instances_error",
			want:        0,
			expectedErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			currentHistory, err := InitHistory(setupStorage(t, tt.initialInstances...))
			require.NoError(t, err, "expected to be able to initialize history without error")

			got, err := currentHistory.LatestInstanceUptimeMinutes(types.DefaultRegistrationID)

			if tt.expectedErr {
				require.Error(t, err)
			}

			assert.Equal(t, tt.want, got)
		})
	}
}

// setupStorage creates storage and seeds it with the given instances.
func setupStorage(t *testing.T, seedInstances ...*Instance) types.KVStore {
	s, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.OsqueryHistoryInstanceStore.String())
	require.NoError(t, err)

	json, err := json.Marshal(seedInstances)
	require.NoError(t, err, "expect no error marshalling instances")

	err = s.Set([]byte(osqueryHistoryInstanceKey), json)
	require.NoError(t, err, "expect no error writing history to bucket")

	return s
}
