//go:build darwin
// +build darwin

package airport

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery/tables/airport/mocks"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_generateAirportData_HappyPath(t *testing.T) {
	t.Parallel()

	type args struct {
		queryContext table.QueryContext
	}
	tests := []struct {
		name             string
		args             args
		execOptionInputs []string
		inputFiles       []string
		outputFile       string
		assertion        assert.ErrorAssertionFunc
	}{
		{
			name: "scan",
			args: args{
				queryContext: table.QueryContext{
					Constraints: map[string]table.ConstraintList{
						"option": {Affinity: "TEXT", Constraints: []table.Constraint{{Operator: table.OperatorEquals, Expression: "scan"}}},
					},
				},
			},
			execOptionInputs: []string{"scan"},
			inputFiles:       []string{"testdata/scan.input.txt"},
			outputFile:       "testdata/scan.output.json",
		},
		{
			name: "scan_with_query",
			args: args{
				queryContext: table.QueryContext{
					Constraints: map[string]table.ConstraintList{
						"option": {Affinity: "TEXT", Constraints: []table.Constraint{{Operator: table.OperatorEquals, Expression: "scan"}}},
						"query":  {Affinity: "TEXT", Constraints: []table.Constraint{{Operator: table.OperatorEquals, Expression: "/SSID"}}},
					},
				},
			},
			execOptionInputs: []string{"scan"},
			inputFiles:       []string{"testdata/scan.input.txt"},
			outputFile:       "testdata/scan_with_query.output.json",
		},
		{
			name: "getinfo",
			args: args{
				queryContext: table.QueryContext{
					Constraints: map[string]table.ConstraintList{
						"option": {Affinity: "TEXT", Constraints: []table.Constraint{{Operator: table.OperatorEquals, Expression: "getinfo"}}},
					},
				},
			},
			execOptionInputs: []string{"getinfo"},
			inputFiles:       []string{"testdata/getinfo.input.txt"},
			outputFile:       "testdata/getinfo.output.json",
		},
		{
			name: "getinfo_and_scan",
			args: args{
				queryContext: table.QueryContext{
					Constraints: map[string]table.ConstraintList{
						"option": {Affinity: "TEXT", Constraints: []table.Constraint{
							{Operator: table.OperatorEquals, Expression: "getinfo"},
							{Operator: table.OperatorEquals, Expression: "scan"},
						}},
					},
				},
			},
			execOptionInputs: []string{"getinfo", "scan"},
			inputFiles:       []string{"testdata/getinfo.input.txt", "testdata/scan.input.txt"},
			outputFile:       "testdata/getinfo_and_scan.output.json",
		},
		{
			name: "getinfo_and_scan_with_query",
			args: args{
				queryContext: table.QueryContext{
					Constraints: map[string]table.ConstraintList{
						"option": {Affinity: "TEXT", Constraints: []table.Constraint{
							{Operator: table.OperatorEquals, Expression: "getinfo"},
							{Operator: table.OperatorEquals, Expression: "scan"},
						}},
						"query": {Affinity: "TEXT", Constraints: []table.Constraint{
							{Operator: table.OperatorEquals, Expression: "/SSID"},
						}},
					},
				},
			},
			execOptionInputs: []string{"getinfo", "scan"},
			inputFiles:       []string{"testdata/getinfo.input.txt", "testdata/scan.input.txt"},
			outputFile:       "testdata/getinfo_and_scan_with_query.output.json",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			executor := &mocks.Executor{}

			for index, inputFile := range tt.inputFiles {
				inputBytes, err := os.ReadFile(inputFile)
				require.NoError(t, err)
				executor.On("Exec", tt.execOptionInputs[index]).Return(inputBytes, nil).Once()
			}

			got, err := generateAirportData(tt.args.queryContext, executor, log.NewNopLogger())
			require.NoError(t, err)

			executor.AssertExpectations(t)

			wantBytes, err := os.ReadFile(tt.outputFile)
			require.NoError(t, err)

			var want []map[string]string
			err = json.Unmarshal(wantBytes, &want)
			require.NoError(t, err)

			assert.ElementsMatch(t, want, got)
		})
	}
}

func Test_generateAirportData_EdgeCases(t *testing.T) {
	t.Parallel()

	type args struct {
		queryContext table.QueryContext
	}
	tests := []struct {
		name       string
		args       args
		execReturn func() ([]byte, error)
		want       []map[string]string
		assertion  assert.ErrorAssertionFunc
	}{
		{
			name: "exec_error",
			args: args{
				queryContext: table.QueryContext{
					Constraints: map[string]table.ConstraintList{
						"option": {Affinity: "TEXT", Constraints: []table.Constraint{{Operator: table.OperatorEquals, Expression: "getinfo"}}},
					},
				},
			},
			execReturn: func() ([]byte, error) {
				return nil, errors.New("exec error")
			},
			assertion: assert.Error,
		},
		{
			name: "no_data",
			args: args{
				queryContext: table.QueryContext{
					Constraints: map[string]table.ConstraintList{
						"option": {Affinity: "TEXT", Constraints: []table.Constraint{{Operator: table.OperatorEquals, Expression: "getinfo"}}},
					},
				},
			},
			execReturn: func() ([]byte, error) {
				return nil, nil
			},
			assertion: assert.NoError,
		},
		{
			name: "blank_data",
			args: args{
				queryContext: table.QueryContext{
					Constraints: map[string]table.ConstraintList{
						"option": {Affinity: "TEXT", Constraints: []table.Constraint{{Operator: table.OperatorEquals, Expression: "getinfo"}}},
					},
				},
			},
			execReturn: func() ([]byte, error) {
				return []byte("   "), nil
			},
			assertion: assert.NoError,
		},
		{
			name: "partial_data",
			args: args{
				queryContext: table.QueryContext{
					Constraints: map[string]table.ConstraintList{
						"option": {Affinity: "TEXT", Constraints: []table.Constraint{{Operator: table.OperatorEquals, Expression: "getinfo"}}},
					},
				},
			},
			execReturn: func() ([]byte, error) {
				return []byte("some data:"), nil
			},
			assertion: assert.NoError,
		},
		{
			name: "unsupported_option",
			args: args{
				queryContext: table.QueryContext{
					Constraints: map[string]table.ConstraintList{
						"option": {Affinity: "TEXT", Constraints: []table.Constraint{{Operator: table.OperatorEquals, Expression: "unsupported"}}},
					},
				},
			},
			execReturn: func() ([]byte, error) {
				return nil, nil
			},
			assertion: assert.Error,
		},
		{
			name: "no_options",
			args: args{},
			execReturn: func() ([]byte, error) {
				return nil, nil
			},
			assertion: assert.Error,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			executor := &mocks.Executor{}

			executor.On("Exec", mock.Anything).Return(tt.execReturn()).Once()

			got, err := generateAirportData(tt.args.queryContext, executor, log.NewNopLogger())
			tt.assertion(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
