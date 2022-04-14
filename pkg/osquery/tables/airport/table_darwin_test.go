//go:build darwin
// +build darwin

package airport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

			inputBytes, err := os.ReadFile(fmt.Sprintf("testdata/%s.input.txt", tt.name))
			require.NoError(t, err)

			got, err := processAirportOutput(bytes.NewReader(inputBytes), tt.args.option, tt.args.queryContext, log.NewNopLogger())
			tt.assertion(t, err)

			wantBytes, err := os.ReadFile(fmt.Sprintf("testdata/%s.output.json", tt.name))
			require.NoError(t, err)

			var want []map[string]string
			err = json.Unmarshal(wantBytes, &want)
			require.NoError(t, err)

			assert.ElementsMatch(t, want, got)
		})
	}
}

func Test_parseAirportOutput_EdgeCases(t *testing.T) {
	t.Parallel()

	type args struct {
		input        io.Reader
		option       string
		queryContext table.QueryContext
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
				input:        strings.NewReader(""),
				option:       "invalid_option",
				queryContext: table.QueryContext{},
			},
			want:      nil,
			assertion: assert.Error,
		},
		{
			name: "only_whitespace_scan",
			args: args{
				input:        strings.NewReader("   "),
				option:       "scan",
				queryContext: table.QueryContext{},
			},
			want:      nil,
			assertion: assert.NoError,
		},
		{
			name: "short_column_data_scan",
			args: args{
				input:        strings.NewReader("column\nrow"),
				option:       "scan",
				queryContext: table.QueryContext{},
			},
			want:      []map[string]string{{"fullkey": "0/column", "key": "column", "option": "scan", "parent": "0", "query": "*", "value": "row"}},
			assertion: assert.NoError,
		},
		{
			name: "only_column_scan",
			args: args{
				input:        strings.NewReader("column1 column2\n"),
				option:       "scan",
				queryContext: table.QueryContext{},
			},
			want:      nil,
			assertion: assert.NoError,
		},
		{
			name: "whitespace_value_scan",
			args: args{
				input:        strings.NewReader("key:\n   "),
				option:       "getinfo",
				queryContext: table.QueryContext{},
			},
			want:      []map[string]string{{"fullkey": "0/key", "key": "key", "option": "getinfo", "parent": "0", "query": "*", "value": ""}},
			assertion: assert.NoError,
		},
		{
			name: "only_whitespace_getinfo",
			args: args{
				input:        strings.NewReader("   "),
				option:       "getinfo",
				queryContext: table.QueryContext{},
			},
			want:      nil,
			assertion: assert.NoError,
		},
		{
			name: "only_key_getinfo",
			args: args{
				input:        strings.NewReader("key: \n"),
				option:       "getinfo",
				queryContext: table.QueryContext{},
			},
			want:      []map[string]string{{"fullkey": "0/key", "key": "key", "option": "getinfo", "parent": "0", "query": "*", "value": ""}},
			assertion: assert.NoError,
		},
		{
			name: "whitespace_row_getinfo",
			args: args{
				input:        strings.NewReader("key: \n   "),
				option:       "getinfo",
				queryContext: table.QueryContext{},
			},
			want:      []map[string]string{{"fullkey": "0/key", "key": "key", "option": "getinfo", "parent": "0", "query": "*", "value": ""}},
			assertion: assert.NoError,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := processAirportOutput(tt.args.input, tt.args.option, tt.args.queryContext, log.NewNopLogger())
			tt.assertion(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
