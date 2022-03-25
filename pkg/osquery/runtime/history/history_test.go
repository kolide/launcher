package history

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewInstance(t *testing.T) { // nolint:paralleltest
	tests := []struct {
		name             string
		setup            func()
		wantNumInstances int
		wantErr          error
	}{
		{
			name:             "zero_value",
			wantNumInstances: 1,
		},
		{
			name: "existing_instances",
			setup: func() {
				currentHistory = &History{
					instances: []*Instance{
						{
							StartTime: "first_start_time",
							ExitTime:  "first_exit_time",
						},
						{
							StartTime: "second_start_time",
							ExitTime:  "second_exit_time",
						},
					},
				}
			},
			wantNumInstances: 3,
		},
		{
			name: "max_instances_reached",
			setup: func() {
				currentHistory = &History{
					instances: []*Instance{
						{}, {}, {}, {}, {}, {}, {}, {}, {},
						{
							ExitTime: "last_exit_time",
						},
					},
				}
			},
			wantNumInstances: 10,
		},
	}
	for _, tt := range tests { // nolint:paralleltest
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}
			t.Cleanup(func() { currentHistory = &History{} })

			_, err := NewInstance()

			assert.Equal(t, tt.wantNumInstances, len(currentHistory.instances))

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
			if err != nil {
				assert.NoError(t, err)
			}
			assert.Equal(t, hostname, currInstance.Hostname)
		})
	}
}

// func TestHistory_CurrentInstanceConnected(t *testing.T) {
// 	t.Parallel()

// 	type fields struct {
// 		instances []*Instance
// 	}
// 	type args struct {
// 		querier *mocks.Querier
// 	}
// 	tests := []struct {
// 		name           string
// 		fields         fields
// 		args           args
// 		querierReturn  func() ([]map[string]string, error)
// 		errString      string
// 		wantInstanceId string
// 		wantVersion    string
// 	}{
// 		{
// 			name: "instance_connected",
// 			fields: fields{
// 				instances: []*Instance{
// 					{
// 						StartTime: "first_start_time",
// 					},
// 				},
// 			},
// 			args: args{querier: &mocks.Querier{}},
// 			querierReturn: func() ([]map[string]string, error) {
// 				return []map[string]string{
// 					{
// 						"instance_id": "00000000-0000-0000-0000-000000000000",
// 						"version":     "0.0.1",
// 					},
// 				}, nil
// 			},
// 			wantInstanceId: "00000000-0000-0000-0000-000000000000",
// 			wantVersion:    "0.0.1",
// 		},
// 		{
// 			name: "query_error",
// 			fields: fields{
// 				instances: []*Instance{{}},
// 			},
// 			args: args{querier: &mocks.Querier{}},
// 			querierReturn: func() ([]map[string]string, error) {
// 				return nil, errors.New("query error")
// 			},
// 			errString: "query error",
// 		},
// 		{
// 			name: "no_rows_error",
// 			fields: fields{
// 				instances: []*Instance{{}},
// 			},
// 			args: args{querier: &mocks.Querier{}},
// 			querierReturn: func() ([]map[string]string, error) {
// 				return make([]map[string]string, 0), nil
// 			},
// 			errString: ExpectedAtLeastOneRowError{}.Error(),
// 		},
// 		{
// 			name: "no_current_instance_error",
// 			args: args{querier: &mocks.Querier{}},
// 			querierReturn: func() ([]map[string]string, error) {
// 				return make([]map[string]string, 0), nil
// 			},
// 			errString: NoCurrentInstanceError{}.Error(),
// 		},
// 	}
// 	for _, tt := range tests {
// 		tt := tt
// 		t.Run(tt.name, func(t *testing.T) {
// 			t.Parallel()

// 			h := &History{
// 				instances: tt.fields.instances,
// 			}

// 			tt.args.querier.On("Query", "select instance_id, version from osquery_info order by start_time limit 1").Return(tt.querierReturn())
// 			err := h.CurrentInstanceConnected(tt.args.querier)

// 			if tt.errString != "" {
// 				assert.EqualError(t, err, tt.errString)
// 			}

// 			if err == nil {
// 				currInstance, err := h.currentInstance()
// 				assert.NoError(t, err, "expect no error getting current instance")

// 				// make sure connected time was set
// 				_, err = time.Parse(time.RFC3339, currInstance.ConnectTime)
// 				assert.NoError(t, err, "expect connected time to be set")

// 				// make sure we record version and instance id
// 				assert.Equal(t, tt.wantInstanceId, currInstance.InstanceId)
// 				assert.Equal(t, tt.wantVersion, currInstance.Version)
// 			}
// 		})
// 	}
// }

// func TestHistory_CurrentInstanceExited(t *testing.T) {
// 	t.Parallel()

// 	type fields struct {
// 		instances []*Instance
// 	}
// 	type args struct {
// 		exitError error
// 	}
// 	tests := []struct {
// 		name      string
// 		fields    fields
// 		args      args
// 		errString string
// 	}{
// 		{
// 			name: "success_no_exit_error",
// 			fields: fields{
// 				instances: []*Instance{{}},
// 			},
// 			args: args{exitError: nil},
// 		},
// 		{
// 			name: "success_exit_error",
// 			fields: fields{
// 				instances: []*Instance{{}},
// 			},
// 			args: args{
// 				exitError: errors.New("some error"),
// 			},
// 		},
// 		{
// 			name: "success_exit_existing_error",
// 			fields: fields{
// 				instances: []*Instance{
// 					{
// 						Error: errors.New("existing error"),
// 					},
// 				},
// 			},
// 			args: args{
// 				exitError: errors.New("additional error error"),
// 			},
// 		},
// 		{
// 			name:      "no_current_instance_error",
// 			errString: NoCurrentInstanceError{}.Error(),
// 		},
// 	}
// 	for _, tt := range tests {
// 		tt := tt
// 		t.Run(tt.name, func(t *testing.T) {
// 			t.Parallel()

// 			h := &History{
// 				instances: tt.fields.instances,
// 			}

// 			err := h.CurrentInstanceExited(tt.args.exitError)

// 			if tt.errString != "" {
// 				assert.EqualError(t, err, tt.errString)
// 			}

// 			if err == nil {
// 				currInstance, err := h.currentInstance()
// 				assert.NoError(t, err, "expect no error getting current instance")

// 				// make sure connected time was set
// 				_, err = time.Parse(time.RFC3339, currInstance.ExitTime)
// 				assert.NoError(t, err, "expect exit time to be set")
// 			}
// 		})
// 	}
// }

func TestGetHistory(t *testing.T) { // nolint:paralleltest
	tests := []struct {
		name      string
		setup     func()
		want      []Instance
		errString string
	}{
		{
			name: "success",
			setup: func() {
				currentHistory = &History{
					instances: []*Instance{
						{
							StartTime: "first_expected_start_time",
						},
						{
							StartTime: "second_expected_start_time",
						},
					},
				}
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
			if tt.setup != nil {
				tt.setup()
			}

			t.Cleanup(func() { currentHistory = &History{} })

			got, err := GetHistory()

			if tt.errString != "" {
				assert.EqualError(t, err, tt.errString)
			}

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCurrentInstance(t *testing.T) { // nolint:paralleltest
	tests := []struct {
		name      string
		setup     func()
		want      Instance
		errString string
	}{
		{
			name: "success",
			setup: func() {
				currentHistory = &History{
					instances: []*Instance{
						{
							StartTime: "first_expected_start_time",
						},
						{
							StartTime: "second_expected_start_time",
						},
					},
				}
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
			if tt.setup != nil {
				tt.setup()
			}

			t.Cleanup(func() { currentHistory = &History{} })

			got, err := LatestInstance()

			if tt.errString != "" {
				assert.EqualError(t, err, tt.errString)
			}

			assert.Equal(t, tt.want, got)
		})
	}
}
