//go:build darwin
// +build darwin

package airport

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery/tables/airport/mocks"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_generateAirportData_HappyPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		sqlEquivalent string

		// this is the query that would be written as part of the sql cmd
		query string

		// these are options that would written as a part of the sql cmd
		// it's required that the options and the executorReturnFilePaths have the same index
		// for example: the index of "scan" in options needs to the the same as the index of "testdata/scan.output.json" in executorReturnFilePaths
		options []string

		// paths to files whose contents are used as the output of the exec call to airport
		executorReturnFilePaths []string

		// path to to the file that is a json of the expected output
		expectedResultsFilePath string
	}{
		{
			sqlEquivalent:           "select * from kolide_airport_util where option = 'scan'",
			options:                 []string{"scan"},
			executorReturnFilePaths: []string{"testdata/scan.input.txt"},
			expectedResultsFilePath: "testdata/scan.output.json",
		},
		{
			sqlEquivalent:           "select * from kolide_airport_util where option = 'scan' and query = '/SSID'",
			options:                 []string{"scan"},
			query:                   "/SSID",
			executorReturnFilePaths: []string{"testdata/scan.input.txt"},
			expectedResultsFilePath: "testdata/scan_with_query.output.json",
		},
		{
			sqlEquivalent:           "select * from kolide_airport_util where option = 'getinfo'",
			options:                 []string{"getinfo"},
			executorReturnFilePaths: []string{"testdata/getinfo.input.txt"},
			expectedResultsFilePath: "testdata/getinfo.output.json",
		},
		{
			sqlEquivalent:           "select * from kolide_airport_util where option in ('getinfo', 'scan')",
			options:                 []string{"getinfo", "scan"},
			executorReturnFilePaths: []string{"testdata/getinfo.input.txt", "testdata/scan.input.txt"},
			expectedResultsFilePath: "testdata/getinfo_and_scan.output.json",
		},
		{
			sqlEquivalent:           "select * from kolide_airport_util where option in ('getinfo', 'scan') and query = '/SSID'",
			options:                 []string{"getinfo", "scan"},
			query:                   "/SSID",
			executorReturnFilePaths: []string{"testdata/getinfo.input.txt", "testdata/scan.input.txt"},
			expectedResultsFilePath: "testdata/getinfo_and_scan_with_query.output.json",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.sqlEquivalent, func(t *testing.T) {
			t.Parallel()

			queryContext := table.QueryContext{
				Constraints: map[string]table.ConstraintList{},
			}

			// add the options constraints
			if tt.options != nil {
				optionConstraints := []table.Constraint{}

				for _, option := range tt.options {
					optionConstraints = append(optionConstraints, table.Constraint{Operator: table.OperatorEquals, Expression: option})
				}

				queryContext.Constraints["option"] = table.ConstraintList{
					Affinity:    "TEXT",
					Constraints: optionConstraints,
				}
			}

			// add the query constraints
			if tt.query != "" {
				queryContext.Constraints["query"] = table.ConstraintList{
					Affinity:    "TEXT",
					Constraints: []table.Constraint{{Operator: table.OperatorEquals, Expression: tt.query}},
				}
			}

			executor := &mocks.Executor{}

			for index, inputFile := range tt.executorReturnFilePaths {
				inputBytes, err := os.ReadFile(inputFile)
				require.NoError(t, err)
				executor.On("Exec", tt.options[index]).Return(inputBytes, nil).Once()
			}

			got, err := generateAirportData(queryContext, executor, log.NewNopLogger())
			require.NoError(t, err)

			executor.AssertExpectations(t)

			wantBytes, err := os.ReadFile(tt.expectedResultsFilePath)
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
			assertion: assert.NoError,
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

func Test_unmarshallGetInfoOutput(t *testing.T) {
	t.Parallel()

	type args struct {
		reader io.Reader
	}
	tests := []struct {
		name string
		args args
		want map[string]interface{}
	}{
		{
			name: "happy_path",
			args: args{
				reader: strings.NewReader("\nagrCtlRSSI: -55\nagrExtRSSI: 0\n"),
			},
			want: map[string]interface{}{
				"agrCtlRSSI": "-55",
				"agrExtRSSI": "0",
			},
		},
		{
			name: "missing_value",
			args: args{
				reader: strings.NewReader("agrCtlRSSI: -55\nagrExtRSSI"),
			},
			want: map[string]interface{}{
				"agrCtlRSSI": "-55",
			},
		},
		{
			name: "no_data",
			args: args{
				reader: strings.NewReader(""),
			},
			want: map[string]interface{}{},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, unmarshallGetInfoOutput(tt.args.reader))
		})
	}
}

func Test_unmarshallScanOuput(t *testing.T) {
	t.Parallel()

	type args struct {
		reader io.Reader
	}
	tests := []struct {
		name string
		args args
		want []map[string]interface{}
	}{
		{
			name: "happy_path",
			args: args{
				reader: strings.NewReader(`
                         SSID BSSID             RSSI CHANNEL HT CC SECURITY (auth/unicast/group)
                i got spaces! a0:a0:a0:a0:a0:a0 -92  108     Y  US WPA(PSK/AES,TKIP/TKIP) RSN(PSK/AES,TKIP/TKIP)
                    no-spaces b1:b1:b1:b1:b1:b1 -91  116     N  EU RSN(PSK/AES/AES)`),
			},
			want: []map[string]interface{}{
				{
					"SSID":                          "i got spaces!",
					"BSSID":                         "a0:a0:a0:a0:a0:a0",
					"RSSI":                          "-92",
					"CHANNEL":                       "108",
					"HT":                            "Y",
					"CC":                            "US",
					"SECURITY (auth/unicast/group)": "WPA(PSK/AES,TKIP/TKIP) RSN(PSK/AES,TKIP/TKIP)",
				},
				{
					"SSID":                          "no-spaces",
					"BSSID":                         "b1:b1:b1:b1:b1:b1",
					"RSSI":                          "-91",
					"CHANNEL":                       "116",
					"HT":                            "N",
					"CC":                            "EU",
					"SECURITY (auth/unicast/group)": "RSN(PSK/AES/AES)",
				},
			},
		},
		{
			name: "no_data",
			args: args{
				reader: strings.NewReader(""),
			},
			want: []map[string]interface{}(nil),
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, unmarshallScanOuput(tt.args.reader))
		})
	}
}
