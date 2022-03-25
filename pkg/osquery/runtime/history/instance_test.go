package history

import (
	"errors"
	"testing"
	"time"

	"github.com/kolide/launcher/pkg/osquery/runtime/history/mocks"
	"github.com/stretchr/testify/assert"
)

func TestInstance_Connected(t *testing.T) {
	t.Parallel()

	// have to declare this up here to use later in comparison
	queryError := errors.New("some query error")

	tests := []struct {
		name           string
		querierReturn  func() ([]map[string]string, error)
		wantVersion    string
		wantInstanceId string
		wantErrReturn  error
	}{
		{
			name: "success",
			querierReturn: func() ([]map[string]string, error) {
				return []map[string]string{
					{
						"instance_id": "00000000-0000-0000-0000-000000000000",
						"version":     "1.0.0",
					},
				}, nil
			},
			wantVersion:    "1.0.0",
			wantInstanceId: "00000000-0000-0000-0000-000000000000",
		},
		{
			name: "query_error",
			querierReturn: func() ([]map[string]string, error) {
				return nil, queryError
			},
			wantErrReturn: queryError,
		},
		{
			name: "no_rows_error",
			querierReturn: func() ([]map[string]string, error) {
				return []map[string]string{}, nil
			},
			wantErrReturn: ExpectedAtLeastOneRowError{},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			i := &Instance{}
			querier := &mocks.Querier{}
			querier.On("Query", "select instance_id, version from osquery_info order by start_time limit 1").Return(tt.querierReturn())

			err := i.Connected(querier)
			assert.Equal(t, tt.wantInstanceId, i.InstanceId)
			assert.Equal(t, tt.wantVersion, i.Version)
			assert.ErrorIs(t, tt.wantErrReturn, err)

			if tt.wantErrReturn == nil {
				// make sure connect time was set
				_, err = time.Parse(time.RFC3339, i.ConnectTime)
				assert.NoError(t, err, "expect connect time to be set")
			}
		})
	}
}

func TestInstance_Exited(t *testing.T) {
	t.Parallel()

	exitError := errors.New("some error")

	type args struct {
		exitError error
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "success",
		},
		{
			name: "success_with_arg",
			args: args{
				exitError: exitError,
			},
			wantErr: exitError,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			i := &Instance{}
			i.Exited(tt.args.exitError)

			assert.ErrorIs(t, tt.wantErr, i.Error)

			// make sure exit time was set
			_, err := time.Parse(time.RFC3339, i.ExitTime)
			assert.NoError(t, err, "expect exit time to be set")
		})
	}
}
