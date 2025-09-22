package observability

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
)

func Test_buildAttributes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		testName       string
		keyVals        []interface{}
		filename       string
		expectedOutput []attribute.KeyValue
	}{
		{
			testName:       "bool",
			keyVals:        []interface{}{"enabled", true},
			filename:       "some/path/to/boolpkg/bool.go",
			expectedOutput: []attribute.KeyValue{attribute.Bool("launcher.boolpkg.enabled", true)},
		},
		{
			testName:       "bool slice",
			keyVals:        []interface{}{"active_states", []bool{true, false, true}},
			filename:       "some/path/to/a/boolpkg/bool.go",
			expectedOutput: []attribute.KeyValue{attribute.BoolSlice("launcher.boolpkg.active_states", []bool{true, false, true})},
		},
		{
			testName:       "int",
			keyVals:        []interface{}{"uptime_sec", 300},
			filename:       "some/path/to/intpkg/int.go",
			expectedOutput: []attribute.KeyValue{attribute.Int("launcher.intpkg.uptime_sec", 300)},
		},
		{
			testName:       "int slice",
			keyVals:        []interface{}{"event_counts", []int{1, 1, 2, 3, 5}},
			filename:       "some/path/to/intpkg/int.go",
			expectedOutput: []attribute.KeyValue{attribute.IntSlice("launcher.intpkg.event_counts", []int{1, 1, 2, 3, 5})},
		},
		{
			testName:       "int64",
			keyVals:        []interface{}{"ts", time.Date(2023, 6, 5, 1, 1, 1, 0, time.UTC).Unix()},
			filename:       "some/path/to/intpkg/int.go",
			expectedOutput: []attribute.KeyValue{attribute.Int64("launcher.intpkg.ts", time.Date(2023, 6, 5, 1, 1, 1, 0, time.UTC).Unix())},
		},
		{
			testName:       "int64 slice",
			keyVals:        []interface{}{"event_ts", []int64{time.Date(2023, 6, 4, 1, 1, 1, 0, time.UTC).Unix(), time.Date(2023, 6, 5, 1, 1, 1, 0, time.UTC).Unix()}},
			filename:       "some/path/to/intpkg/int.go",
			expectedOutput: []attribute.KeyValue{attribute.Int64Slice("launcher.intpkg.event_ts", []int64{time.Date(2023, 6, 4, 1, 1, 1, 0, time.UTC).Unix(), time.Date(2023, 6, 5, 1, 1, 1, 0, time.UTC).Unix()})},
		},
		{
			testName:       "float64",
			keyVals:        []interface{}{"some_metric", 3.5},
			filename:       "some/path/to/floatpkg/float.go",
			expectedOutput: []attribute.KeyValue{attribute.Float64("launcher.floatpkg.some_metric", 3.5)},
		},
		{
			testName:       "float64 slice",
			keyVals:        []interface{}{"some_metrics", []float64{1.5, 2.5, 3.4}},
			filename:       "some/path/to/floatpkg/float.go",
			expectedOutput: []attribute.KeyValue{attribute.Float64Slice("launcher.floatpkg.some_metrics", []float64{1.5, 2.5, 3.4})},
		},
		{
			testName:       "string",
			keyVals:        []interface{}{"table_name", "kolide_table"},
			filename:       "some/path/to/stringpkg/string.go",
			expectedOutput: []attribute.KeyValue{attribute.String("launcher.stringpkg.table_name", "kolide_table")},
		},
		{
			testName:       "string slice",
			keyVals:        []interface{}{"args", []string{"-f", "some_file.txt"}},
			filename:       "some/path/to/stringpkg/string.go",
			expectedOutput: []attribute.KeyValue{attribute.StringSlice("launcher.stringpkg.args", []string{"-f", "some_file.txt"})},
		},
		{
			testName:       "no caller",
			keyVals:        []interface{}{"active", false},
			filename:       "",
			expectedOutput: []attribute.KeyValue{attribute.Bool("launcher.unknown.active", false)},
		},
		{
			testName:       "no keyvals",
			keyVals:        []interface{}{},
			filename:       "some/path/to/a/package",
			expectedOutput: []attribute.KeyValue{},
		},
		{
			testName:       "unsupported key",
			keyVals:        []interface{}{1, complex64(2)},
			filename:       "some/path/to/a/package",
			expectedOutput: []attribute.KeyValue{attribute.String("bad key type int: 1", "(2+0i)")},
		},
		{
			testName:       "unsupported value",
			keyVals:        []interface{}{"something", uint8(2)},
			filename:       "some/path/to/a/package",
			expectedOutput: []attribute.KeyValue{attribute.String("launcher.a.something", "unsupported value of type uint8: 2")},
		},
		{
			testName: "multiple keyvals",
			keyVals: []interface{}{
				"enabled", false,
				"uptime_sec", 60,
				"binary", "/usr/sbin/softwareupdate",
				"args", []string{"--no-scan"},
			},
			filename: "some/path/to/multiple/mult.go",
			expectedOutput: []attribute.KeyValue{
				attribute.Bool("launcher.multiple.enabled", false),
				attribute.Int("launcher.multiple.uptime_sec", 60),
				attribute.String("launcher.multiple.binary", "/usr/sbin/softwareupdate"),
				attribute.StringSlice("launcher.multiple.args", []string{"--no-scan"}),
			},
		},
	}

	for _, tt := range testCases {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

			result := buildAttributes(tt.filename, tt.keyVals...)
			require.Equal(t, tt.expectedOutput, result)
		})
	}
}
