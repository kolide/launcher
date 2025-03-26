package history

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/osquery/runtime/history/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewInstance(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		// registrationId will automatically be added to any new instances (if missing in test case) before seeding
		registrationId   string
		initialInstances []*instance
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
			initialInstances: []*instance{
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
			initialInstances: []*instance{
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
		initialInstances []*instance
		want             []map[string]string
		errString        string
	}{
		{
			name: "success",
			initialInstances: []*instance{
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
		initialInstances []*instance
		want             *instance
		errString        string
	}{
		{
			name: "success",
			initialInstances: []*instance{
				{
					StartTime:      "first_expected_start_time",
					RegistrationId: types.DefaultRegistrationID,
				},
				{
					StartTime:      "second_expected_start_time",
					RegistrationId: types.DefaultRegistrationID,
				},
			},
			want: &instance{
				StartTime:      "second_expected_start_time",
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
		initialInstances []*instance
		want             map[string]string
		errString        string
	}{
		{
			name:           "success",
			registrationID: "test",
			initialInstances: []*instance{
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
				"connect_time":    "",
				"errors":          "",
				"exit_time":       "",
				"hostname":        "",
				"instance_id":     "",
				"instance_run_id": "",
				"registration_id": "test",
				"start_time":      "third_expected_start_time",
				"version":         "",
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
			initialInstances: []*instance{
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
		registrationId   string
		initialInstances []*instance
		want             int64
		expectedErr      bool
	}{
		{
			name:           "success",
			registrationId: types.DefaultRegistrationID,
			initialInstances: []*instance{
				{
					RegistrationId: types.DefaultRegistrationID,
					StartTime:      time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339),
				},
				{
					RegistrationId: types.DefaultRegistrationID,
					StartTime:      time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339),
				},
			},
			want:        10,
			expectedErr: false,
		},
		{
			name:           "success_different_registration_ids",
			registrationId: "notTheDefault",
			initialInstances: []*instance{
				{
					RegistrationId: types.DefaultRegistrationID,
					StartTime:      time.Now().UTC().Add(-45 * time.Minute).Format(time.RFC3339),
				},
				{
					RegistrationId: "notTheDefault",
					StartTime:      time.Now().UTC().Add(-40 * time.Minute).Format(time.RFC3339),
				},
				{
					RegistrationId: types.DefaultRegistrationID,
					StartTime:      time.Now().UTC().Add(-35 * time.Minute).Format(time.RFC3339),
				},
				{
					RegistrationId: "notTheDefault",
					StartTime:      time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339),
				},
				{
					RegistrationId: "notTheDefault",
					StartTime:      time.Now().UTC().Add(-25 * time.Minute).Format(time.RFC3339),
				},
			},
			want:        25,
			expectedErr: false,
		},
		{
			name:           "no_instances_error",
			registrationId: types.DefaultRegistrationID,
			want:           0,
			expectedErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			currentHistory, err := InitHistory(setupStorage(t, tt.initialInstances...))
			require.NoError(t, err, "expected to be able to initialize history without error")

			got, err := currentHistory.LatestInstanceUptimeMinutes(tt.registrationId)

			if tt.expectedErr {
				require.Error(t, err)
			}

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLatestInstanceId(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		initialInstances []*instance
		registrationId   string
		want             string
		expectedErr      bool
	}{
		{
			name:           "success_same_registration_ids",
			registrationId: types.DefaultRegistrationID,
			initialInstances: []*instance{
				{
					RegistrationId: types.DefaultRegistrationID,
					StartTime:      time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339),
					InstanceId:     ulid.New(),
				},
				{
					RegistrationId: types.DefaultRegistrationID,
					StartTime:      time.Now().UTC().Add(-20 * time.Minute).Format(time.RFC3339),
					InstanceId:     ulid.New(),
				},
				{
					RegistrationId: types.DefaultRegistrationID,
					StartTime:      time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339),
					InstanceId:     "9b093496-9999-9999-ab70-6ceb816b8775",
				},
			},
			want:        "9b093496-9999-9999-ab70-6ceb816b8775",
			expectedErr: false,
		},
		{
			name:           "success_different_registration_ids",
			registrationId: types.DefaultRegistrationID,
			initialInstances: []*instance{
				{
					RegistrationId: types.DefaultRegistrationID,
					StartTime:      time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339),
					InstanceId:     ulid.New(),
				},
				{
					RegistrationId: types.DefaultRegistrationID,
					StartTime:      time.Now().UTC().Add(-20 * time.Minute).Format(time.RFC3339),
					InstanceId:     ulid.New(),
				},
				{
					RegistrationId: types.DefaultRegistrationID,
					StartTime:      time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339),
					InstanceId:     "9b093496-9999-9999-ab70-6ceb816b8775",
				},
				{
					RegistrationId: ulid.New(),
					StartTime:      time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339),
					InstanceId:     ulid.New(),
				},
				{
					RegistrationId: ulid.New(),
					StartTime:      time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339),
					InstanceId:     ulid.New(),
				},
			},
			want:        "9b093496-9999-9999-ab70-6ceb816b8775",
			expectedErr: false,
		},
		{
			name:        "no_instances_error",
			want:        "",
			expectedErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			currentHistory, err := InitHistory(setupStorage(t, tt.initialInstances...))
			require.NoError(t, err, "expected to be able to initialize history without error")

			got, err := currentHistory.LatestInstanceId(types.DefaultRegistrationID)

			if tt.expectedErr {
				require.Error(t, err)
			}

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSetConnected(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name               string
		initialInstances   []*instance
		querierReturn      func() ([]map[string]string, error)
		runId              string
		expectedInstanceId string
		expectedErr        error
	}{
		{
			name:               "success_same_registration_ids",
			runId:              "99999999-9999-9999-9999-999999999999",
			expectedInstanceId: "00000000-0000-0000-0000-000000000000",
			querierReturn: func() ([]map[string]string, error) {
				return []map[string]string{
					{
						"instance_id": "00000000-0000-0000-0000-000000000000",
						"version":     "5.16.0",
					},
				}, nil
			},
			initialInstances: []*instance{
				{
					RegistrationId: types.DefaultRegistrationID,
					StartTime:      time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339),
					RunId:          ulid.New(),
				},
				{
					RegistrationId: types.DefaultRegistrationID,
					StartTime:      time.Now().UTC().Add(-20 * time.Minute).Format(time.RFC3339),
					RunId:          "99999999-9999-9999-9999-999999999999",
				},
				{
					RegistrationId: types.DefaultRegistrationID,
					StartTime:      time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339),
					RunId:          ulid.New(),
				},
			},
			expectedErr: nil,
		},
		{
			name:               "success_different_registration_ids",
			runId:              "99999999-9999-9999-9999-999999999999",
			expectedInstanceId: "00000000-0000-0000-0000-000000000000",
			querierReturn: func() ([]map[string]string, error) {
				return []map[string]string{
					{
						"instance_id": "00000000-0000-0000-0000-000000000000",
						"version":     "5.16.0",
					},
				}, nil
			},
			initialInstances: []*instance{
				{
					RegistrationId: types.DefaultRegistrationID,
					StartTime:      time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339),
					RunId:          ulid.New(),
				},
				{
					RegistrationId: types.DefaultRegistrationID,
					StartTime:      time.Now().UTC().Add(-20 * time.Minute).Format(time.RFC3339),
					RunId:          ulid.New(),
				},
				{
					RegistrationId: types.DefaultRegistrationID,
					StartTime:      time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339),
					RunId:          "99999999-9999-9999-9999-999999999999",
				},
				{
					RegistrationId: ulid.New(),
					StartTime:      time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339),
					RunId:          ulid.New(),
				},
				{
					RegistrationId: ulid.New(),
					StartTime:      time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339),
					RunId:          ulid.New(),
				},
			},
			expectedErr: nil,
		},
		{
			name:        "no_instances_error",
			expectedErr: NoInstancesError{},
		},
		{
			name:  "querier_error",
			runId: "99999999-9999-9999-9999-999999999999",
			querierReturn: func() ([]map[string]string, error) {
				return nil, ExpectedAtLeastOneRowError{}
			},
			initialInstances: []*instance{
				{
					RegistrationId: types.DefaultRegistrationID,
					StartTime:      time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339),
					RunId:          "99999999-9999-9999-9999-999999999999",
				},
			},
			expectedErr: ExpectedAtLeastOneRowError{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			osqHistory, err := InitHistory(setupStorage(t, tt.initialInstances...))
			require.NoError(t, err, "expected to be able to initialize history without error")
			querier := &mocks.Querier{}
			if tt.querierReturn != nil {
				querier.On("Query", "select instance_id, version from osquery_info order by start_time limit 1").Return(tt.querierReturn()).Once()
			}

			err = osqHistory.SetConnected(tt.runId, querier)
			if tt.expectedErr == nil {
				historyStats, err := osqHistory.GetHistory()
				require.NoError(t, err, "expected to be able to gather history after setting connected")
				foundRunId := false
				for _, stats := range historyStats {
					// we expect all stats to contain the connect time and instance id fields,
					// but only our tested runId to have set those values
					require.Contains(t, stats, "connect_time")
					require.Contains(t, stats, "instance_id")
					if runId, ok := stats["instance_run_id"]; ok && runId == tt.runId {
						foundRunId = true
						require.NotEmpty(t, stats["connect_time"])
						require.Equal(t, tt.expectedInstanceId, stats["instance_id"])
					} else {
						require.Empty(t, stats["connect_time"])
						require.Empty(t, stats["instance_id"])
					}
				}

				require.True(t, foundRunId, "tested run id was not present in history stats")
			} else {
				require.Error(t, err)
				require.True(t, errors.Is(err, tt.expectedErr))
			}
		})
	}
}

func TestSetExited(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		initialInstances []*instance
		runId            string
		expectedErr      error
		exitErr          error
	}{
		{
			name:  "success_same_registration_ids",
			runId: "99999999-9999-9999-9999-999999999999",
			initialInstances: []*instance{
				{
					RegistrationId: types.DefaultRegistrationID,
					StartTime:      time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339),
					RunId:          ulid.New(),
				},
				{
					RegistrationId: types.DefaultRegistrationID,
					StartTime:      time.Now().UTC().Add(-20 * time.Minute).Format(time.RFC3339),
					RunId:          "99999999-9999-9999-9999-999999999999",
				},
				{
					RegistrationId: types.DefaultRegistrationID,
					StartTime:      time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339),
					RunId:          ulid.New(),
				},
			},
			expectedErr: nil,
			exitErr:     errors.New("unexpected exit"),
		},
		{
			name:  "success_different_registration_ids",
			runId: "99999999-9999-9999-9999-999999999999",
			initialInstances: []*instance{
				{
					RegistrationId: types.DefaultRegistrationID,
					StartTime:      time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339),
					RunId:          ulid.New(),
				},
				{
					RegistrationId: types.DefaultRegistrationID,
					StartTime:      time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339),
					RunId:          "99999999-9999-9999-9999-999999999999",
				},
				{
					RegistrationId: ulid.New(),
					StartTime:      time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339),
					RunId:          ulid.New(),
				},
				{
					RegistrationId: ulid.New(),
					StartTime:      time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339),
					RunId:          ulid.New(),
				},
			},
			expectedErr: nil,
			exitErr:     errors.New("unexpected exit"),
		},
		{
			name:        "no_instances_error",
			expectedErr: NoInstancesError{},
			exitErr:     errors.New("unexpected exit"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			osqHistory, err := InitHistory(setupStorage(t, tt.initialInstances...))
			require.NoError(t, err, "expected to be able to initialize history without error")

			err = osqHistory.SetExited(tt.runId, tt.exitErr)
			if tt.expectedErr == nil {
				historyStats, err := osqHistory.GetHistory()
				require.NoError(t, err, "expected to be able to gather history after setting connected")
				foundRunId := false
				for _, stats := range historyStats {
					// we expect all stats to contain the exit time and errors fields,
					// but only our tested runId to have set those values
					require.Contains(t, stats, "exit_time")
					require.Contains(t, stats, "errors")
					if runId, ok := stats["instance_run_id"]; ok && runId == tt.runId {
						foundRunId = true
						require.NotEmpty(t, stats["exit_time"])
						require.Equal(t, tt.exitErr.Error(), stats["errors"])
					} else {
						require.Empty(t, stats["exit_time"])
						require.Empty(t, stats["errors"])
					}
				}

				require.True(t, foundRunId, "tested run id was not present in history stats")
			} else {
				require.Error(t, err)
				require.True(t, errors.Is(err, tt.expectedErr))
			}
		})
	}
}

// setupStorage creates storage and seeds it with the given instances.
func setupStorage(t *testing.T, seedInstances ...*instance) types.KVStore {
	s, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.OsqueryHistoryInstanceStore.String())
	require.NoError(t, err)

	json, err := json.Marshal(seedInstances)
	require.NoError(t, err, "expect no error marshalling instances")

	err = s.Set([]byte(osqueryHistoryInstanceKey), json)
	require.NoError(t, err, "expect no error writing history to bucket")

	return s
}
