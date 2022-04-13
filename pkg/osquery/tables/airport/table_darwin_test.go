//go:build darwin
// +build darwin

package airport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/stretchr/testify/assert"
)

func Test_parseAirportOutput_HappyPath(t *testing.T) {
	t.Parallel()

	type args struct {
		option       string
		queryContext table.QueryContext
	}
	tests := []struct {
		name      string
		args      args
		assertion assert.ErrorAssertionFunc
	}{
		{
			name: "scan",
			args: args{
				option:       "scan",
				queryContext: table.QueryContext{},
			},
			assertion: assert.NoError,
		},
		{
			name: "getinfo",
			args: args{
				option:       "getinfo",
				queryContext: table.QueryContext{},
			},
			assertion: assert.NoError,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			inputFile, err := os.Open(fmt.Sprintf("testdata/%s.input.txt", tt.name))
			assert.NoError(t, err)
			defer inputFile.Close()

			input, err := ioutil.ReadAll(inputFile)
			assert.NoError(t, err)

			got, err := processAirportOutput(bytes.NewReader(input), tt.args.option, tt.args.queryContext, log.NewNopLogger())
			tt.assertion(t, err)

			wantFile, err := os.Open(fmt.Sprintf("testdata/%s.output.json", tt.name))
			assert.NoError(t, err)
			defer wantFile.Close()

			wantBytes, err := ioutil.ReadAll(wantFile)
			assert.NoError(t, err)

			var want []map[string]string
			err = json.Unmarshal(wantBytes, &want)
			assert.NoError(t, err)

			assert.ElementsMatch(t, want, got)
		})
	}
}

func Test_parseAirportOutput_EdgeCases(t *testing.T) {
	t.Parallel()

	type args struct {
		output       io.Reader
		option       string
		queryContext table.QueryContext
		logger       log.Logger
	}
	tests := []struct {
		name      string
		args      args
		want      []map[string]string
		assertion assert.ErrorAssertionFunc
	}{
		{
			name: "invalid_option",
			args: args{
				output:       strings.NewReader(""),
				option:       "invalid_option",
				queryContext: table.QueryContext{},
				logger:       log.NewNopLogger(),
			},
			want:      nil,
			assertion: assert.Error,
		},
		{
			name: "only_whitespace_scan",
			args: args{
				output:       strings.NewReader("   "),
				option:       "scan",
				queryContext: table.QueryContext{},
				logger:       log.NewNopLogger(),
			},
			want:      nil,
			assertion: assert.NoError,
		},
		{
			name: "short_column_data_scan",
			args: args{
				output:       strings.NewReader("column\nrow"),
				option:       "scan",
				queryContext: table.QueryContext{},
				logger:       log.NewNopLogger(),
			},
			want:      []map[string]string{{"fullkey": "0/column", "key": "column", "option": "scan", "parent": "0", "query": "*", "value": "row"}},
			assertion: assert.NoError,
		},
		{
			name: "only_column_scan",
			args: args{
				output:       strings.NewReader("column1 column2\n"),
				option:       "scan",
				queryContext: table.QueryContext{},
				logger:       log.NewNopLogger(),
			},
			want:      nil,
			assertion: assert.NoError,
		},
		{
			name: "whitespace_value_scan",
			args: args{
				output:       strings.NewReader("key:\n   "),
				option:       "getinfo",
				queryContext: table.QueryContext{},
				logger:       log.NewNopLogger(),
			},
			want:      []map[string]string{{"fullkey": "0/key", "key": "key", "option": "getinfo", "parent": "0", "query": "*", "value": ""}},
			assertion: assert.NoError,
		},
		{
			name: "only_whitespace_getinfo",
			args: args{
				output:       strings.NewReader("   "),
				option:       "getinfo",
				queryContext: table.QueryContext{},
				logger:       log.NewNopLogger(),
			},
			want:      nil,
			assertion: assert.NoError,
		},
		{
			name: "only_key_getinfo",
			args: args{
				output:       strings.NewReader("key: \n"),
				option:       "getinfo",
				queryContext: table.QueryContext{},
				logger:       log.NewNopLogger(),
			},
			want:      []map[string]string{{"fullkey": "0/key", "key": "key", "option": "getinfo", "parent": "0", "query": "*", "value": ""}},
			assertion: assert.NoError,
		},
		{
			name: "whitespace_row_getinfo",
			args: args{
				output:       strings.NewReader("key: \n   "),
				option:       "getinfo",
				queryContext: table.QueryContext{},
				logger:       log.NewNopLogger(),
			},
			want:      []map[string]string{{"fullkey": "0/key", "key": "key", "option": "getinfo", "parent": "0", "query": "*", "value": ""}},
			assertion: assert.NoError,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := processAirportOutput(tt.args.output, tt.args.option, tt.args.queryContext, tt.args.logger)
			tt.assertion(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
