package osquerylogs

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractOsqueryCaller(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		log      string
		expected string
	}{
		{
			`I1101 19:21:40.292618 84815872 distributed.cpp:133] Executing distributed query: kolide:populate:practices:1: SELECT COUNT(*) AS result FROM (select * from time);`,
			`distributed.cpp:133`,
		},
		{
			`E1201 08:21:54.254618 84815872 foobar.m:47] Penguin`,
			`foobar.m:47`,
		},
		{
			`E1201 08:21:54.254618 84815872 unknown] Penguin`,
			``,
		},
		{
			`Just plain bad`,
			``,
		},
	}

	for _, tt := range testCases {
		tt := tt
		t.Run("", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, extractOsqueryCaller(tt.log))
		})
	}
}

func Test_extractLogLevel(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName     string
		defaultLogLevel  slog.Level
		msg              string
		expectedLogLevel slog.Level
	}{
		{
			testCaseName:     "error",
			defaultLogLevel:  slog.LevelDebug,
			msg:              "E0731 11:00:07.412808 135955776 registry_factory.cpp:188] sql registry sql plugin caused exception: map::at:  key not found",
			expectedLogLevel: slog.LevelError,
		},
		{
			testCaseName:     "warn",
			defaultLogLevel:  slog.LevelDebug,
			msg:              "W0801 10:07:54.639108 1888202752 mdfind.mm:74] Could not execute mdfind query",
			expectedLogLevel: slog.LevelWarn,
		},
		{
			testCaseName:     "info",
			defaultLogLevel:  slog.LevelDebug,
			msg:              "I0804 10:12:20.279402 1880748032 config.cpp:1334] Refreshing configuration state",
			expectedLogLevel: slog.LevelInfo,
		},
		{
			testCaseName:     "non-match from END in SQL",
			defaultLogLevel:  slog.LevelDebug,
			msg:              "END AS some_name",
			expectedLogLevel: slog.LevelDebug,
		},
		{
			testCaseName:     "non-match from ELSE in SQL",
			defaultLogLevel:  slog.LevelDebug,
			msg:              "ELSE 'false' END AS some_condition",
			expectedLogLevel: slog.LevelDebug,
		},
		{
			testCaseName:     "non-match from WHEN in SQL",
			defaultLogLevel:  slog.LevelInfo,
			msg:              "WHEN x = 100",
			expectedLogLevel: slog.LevelInfo,
		},
		{
			testCaseName:     "non-match from IN in SQL",
			defaultLogLevel:  slog.LevelDebug,
			msg:              "IN (100, 200)",
			expectedLogLevel: slog.LevelDebug,
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			adapter := &OsqueryLogAdapter{
				level: tt.defaultLogLevel,
			}

			require.Equal(t, tt.expectedLogLevel, adapter.extractLogLevel(tt.msg))
		})
	}
}
