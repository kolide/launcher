package osquery_instance_history

import (
	"errors"
	"reflect"
	"testing"

	"github.com/kolide/launcher/pkg/osquery/runtime/osquery_instance_history/mocks"
	"github.com/stretchr/testify/require"
)

func TestInstanceStarted(t *testing.T) { // nolint:paralleltest
	tests := []struct {
		name         string
		setup        func()
		wantErr      bool
		numInstances int
	}{
		{
			name:         "happy_path_empty",
			setup:        func() {},
			wantErr:      false,
			numInstances: 1,
		},
		{
			name: "happy_path_new_current_instance",
			setup: func() {
				currentHistory = &history{
					instances: []*Instance{
						{
							StartTime: "first",
							ExitTime:  "second",
						},
					},
				}
			},
			wantErr:      false,
			numInstances: 2,
		},
		{
			name: "max_instances_reached",
			setup: func() {
				currentHistory.instances = make([]*Instance, maxInstances+1)
			},
			wantErr:      false,
			numInstances: maxInstances,
		},
		{
			name: "current_instance_not_exited",
			setup: func() {
				currentHistory = &history{
					instances: []*Instance{
						{
							StartTime: "whatever",
						},
					},
				}
			},
			wantErr:      true,
			numInstances: 1,
		},
	}
	for _, tt := range tests { // nolint:paralleltest
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			t.Cleanup(func() {
				currentHistory = &history{}
			})

			if err := InstanceStarted(); (err != nil) != tt.wantErr {
				t.Errorf("InstanceStarted() error = %v, wantErr %v", err, tt.wantErr)
			}

			require.Equal(t, tt.numInstances, len(currentHistory.instances))
		})
	}
}

func TestInstanceConnected(t *testing.T) { // nolint:paralleltest
	type args struct {
		querier *mocks.Querier
	}
	tests := []struct {
		name          string
		args          args
		setup         func()
		querierReturn func() ([]map[string]string, error)
		wantErr       bool
	}{
		{
			name: "happy_path",
			args: args{querier: &mocks.Querier{}},
			setup: func() {
				currentHistory = &history{
					instances: []*Instance{
						{
							StartTime: "first",
						},
					},
				}
			},
			querierReturn: func() ([]map[string]string, error) {
				return []map[string]string{
					{
						"instance_id": "00000000-0000-0000-0000-000000000000",
					},
				}, nil
			},
			wantErr: false,
		},
		{
			name: "query_error",
			args: args{querier: &mocks.Querier{}},
			setup: func() {
				currentHistory = &history{
					instances: []*Instance{
						{
							StartTime: "first",
						},
					},
				}
			},
			querierReturn: func() ([]map[string]string, error) {
				return nil, errors.New("query error")
			},
			wantErr: true,
		},
		{
			name: "error_no_results",
			args: args{querier: &mocks.Querier{}},
			setup: func() {
				currentHistory = &history{
					instances: []*Instance{
						{
							StartTime: "first",
						},
					},
				}
			},
			querierReturn: func() ([]map[string]string, error) {
				return []map[string]string{}, nil
			},
			wantErr: true,
		},
		{
			name:          "error_no_current_instance",
			args:          args{querier: &mocks.Querier{}},
			setup:         func() {},
			querierReturn: func() ([]map[string]string, error) { return nil, nil },
			wantErr:       true,
		},
	}
	for _, tt := range tests { // nolint:paralleltest
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			t.Cleanup(func() {
				currentHistory = &history{}
			})

			tt.args.querier.On("Query", "select instance_id from osquery_info order by start_time limit 1").Return(tt.querierReturn())

			if err := InstanceConnected(tt.args.querier); (err != nil) != tt.wantErr {
				t.Errorf("InstanceConnected() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestInstanceExited(t *testing.T) { // nolint:paralleltest
	type args struct {
		exitError error
	}
	tests := []struct {
		name    string
		args    args
		setup   func()
		wantErr bool
	}{
		{
			name: "happy_path",
			args: args{exitError: nil},
			setup: func() {
				currentHistory = &history{
					instances: []*Instance{
						{
							StartTime: "first",
							ExitTime:  "second",
						},
					},
				}
			},
			wantErr: false,
		},
		{
			name: "happy_path_with_error",
			args: args{exitError: errors.New("some exit error")},
			setup: func() {
				currentHistory = &history{
					instances: []*Instance{
						{
							StartTime: "first",
							ExitTime:  "second",
						},
					},
				}
			},
			wantErr: false,
		},
		{
			name: "happy_path_existing_instance_error",
			args: args{exitError: errors.New("some exit error")},
			setup: func() {
				currentHistory = &history{
					instances: []*Instance{
						{
							StartTime: "first",
							ExitTime:  "second",
							Error:     errors.New("im an existing error"),
						},
					},
				}
			},
			wantErr: false,
		},
		{
			name:    "error_no_current_instance",
			args:    args{},
			setup:   func() {},
			wantErr: true,
		},
	}
	for _, tt := range tests { // nolint:paralleltest
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			t.Cleanup(func() {
				currentHistory = &history{}
			})

			if err := InstanceExited(tt.args.exitError); (err != nil) != tt.wantErr {
				t.Errorf("InstanceExited() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetHistory(t *testing.T) { // nolint:paralleltest
	tests := []struct {
		name  string
		setup func()
		want  []Instance
	}{
		{
			name: "happy_path",
			setup: func() {
				currentHistory = &history{
					instances: []*Instance{
						{
							StartTime: "a",
							ExitTime:  "a",
						},
						{
							StartTime: "b",
							ExitTime:  "b",
						},
					},
				}
			},
			want: []Instance{
				{
					StartTime: "a",
					ExitTime:  "a",
				},
				{
					StartTime: "b",
					ExitTime:  "b",
				},
			},
		},
	}
	for _, tt := range tests { // nolint:paralleltest
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			t.Cleanup(func() {
				currentHistory = &history{}
			})
			if got := GetHistory(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetHistory() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCurrentInstance(t *testing.T) { // nolint:paralleltest
	tests := []struct {
		name    string
		setup   func()
		want    Instance
		wantErr bool
	}{
		{
			name: "happy_path",
			setup: func() {
				currentHistory = &history{
					instances: []*Instance{
						{
							StartTime: "a",
							ExitTime:  "a",
						},
						{
							StartTime: "b",
							ExitTime:  "b",
						},
					},
				}
			},
			want: Instance{
				StartTime: "b",
				ExitTime:  "b",
			},
			wantErr: false,
		},
		{
			name: "happy_path_error_on_instance",
			setup: func() {
				currentHistory = &history{
					instances: []*Instance{
						{
							StartTime: "a",
							ExitTime:  "a",
						},
						{
							StartTime: "b",
							ExitTime:  "b",
							Error:     errors.New("existing error"),
						},
					},
				}
			},
			want: Instance{
				StartTime: "b",
				ExitTime:  "b",
				Error:     errors.New("existing error"),
			},
			wantErr: false,
		},
		{
			name:    "error_no_current_instance",
			setup:   func() {},
			want:    Instance{},
			wantErr: true,
		},
	}
	for _, tt := range tests { // nolint:paralleltest
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			t.Cleanup(func() {
				currentHistory = &history{}
			})

			got, err := CurrentInstance()
			if (err != nil) != tt.wantErr {
				t.Errorf("CurrentInstance() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CurrentInstance() = %v, want %v", got, tt.want)
			}
		})
	}
}
