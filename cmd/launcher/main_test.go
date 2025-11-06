package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_commandExpectedToExit(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName string
		osArgs       []string
		expectedExit bool
	}{
		{
			testCaseName: "launcher alone",
			osArgs: []string{
				"/some/path/to/launcher",
			},
			expectedExit: false,
		},
		{
			testCaseName: "launcher with config",
			osArgs: []string{
				"/some/path/to/launcher",
				"-config",
				"/some/path/to/config",
			},
			expectedExit: false,
		},
		{
			testCaseName: "launcher.exe svc",
			osArgs: []string{
				"/some/path/to/launcher.exe",
				"svc",
				"/some/path/to/config",
			},
			expectedExit: false,
		},
		{
			testCaseName: "launcher subcommand doctor",
			osArgs: []string{
				"/some/path/to/launcher",
				"doctor",
				"-config",
				"/some/path/to/config",
			},
			expectedExit: true,
		},
		{
			testCaseName: "launcher subcommand flare",
			osArgs: []string{
				"/some/path/to/launcher",
				"flare",
			},
			expectedExit: true,
		},
		{
			testCaseName: "launcher subcommand version",
			osArgs: []string{
				"/some/path/to/launcher",
				"version",
			},
			expectedExit: true,
		},
		{
			testCaseName: "launcher subcommand version flag",
			osArgs: []string{
				"/some/path/to/launcher",
				"--version",
				"--config",
				"/some/path/to/config",
			},
			expectedExit: true,
		},
		{
			testCaseName: "launcher subcommand version flag, single dash",
			osArgs: []string{
				"/some/path/to/launcher",
				"-version",
			},
			expectedExit: true,
		},
		{
			testCaseName: "launcher subcommand interactive",
			osArgs: []string{
				"/some/path/to/launcher",
				"interactive",
				"--config",
				"/some/path/to/config",
			},
			expectedExit: true,
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.expectedExit, commandExpectedToExit(tt.osArgs))
		})
	}
}
