// +build windows

package wmi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuery(t *testing.T) {
	t.Parallel()

	// If you want a logger for debugging, add it in to the ctx
	ctx := context.TODO()

	var tests = []struct {
		name       string
		class      string
		properties []string
		options    []Option
		minRows    int
		noData     bool
		err        bool
	}{
		{
			name:       "simple operating system query",
			class:      "Win32_OperatingSystem",
			properties: []string{"name", "version"},
			minRows:    1,
		},
		{
			name:       "queries with non-string types",
			class:      "Win32_OperatingSystem",
			properties: []string{"InstallDate", "primary"},
			minRows:    1,
		},
		{
			name:       "process query",
			class:      "WIN32_process",
			properties: []string{"Caption", "CommandLine", "CreationDate", "Name", "Handle", "ReadTransferCount"},
			minRows:    10,
		},
		{
			name:       "semicolon in class name",
			class:      "Win32_OperatingSystem;",
			properties: []string{"name", "version"},
			noData:     true,
		},
		{
			name:       "unknown classname",
			class:      "Win32_MadeUp",
			properties: []string{"name"},
			noData:     true,
		},
		{
			name:       "semicolon in properties",
			class:      "Win32_OperatingSystem",
			properties: []string{"ver;sion"},
			noData:     true,
		},
		{
			name:       "unknown properties",
			class:      "Win32_OperatingSystem;",
			properties: []string{"madeup1", "imaginary2"},
			noData:     true,
		},
		{
			name:       "blank namespace",
			class:      "Win32_OperatingSystem",
			properties: []string{"name", "version"},
			options:    ConnectNamespace(""),
			minRows:    1,
		},
		{
			name:       "default namespace",
			class:      "Win32_OperatingSystem",
			properties: []string{"name", "version"},
			options:    ConnectNamespace(`root\cimv2`),
			minRows:    1,
		},
		{
			name:       "unknown namespace",
			class:      "Win32_OperatingSystem",
			properties: []string{"name", "version"},
			options:    ConnectNamespace(`no\such\namespace`),
			minRows:    1,
		},
		{
			name:       "different namespace",
			class:      "MSKeyboard_PortInformation",
			properties: []string{"ConnectorType", "FunctionKeys", "Indicators"},
			options:    ConnectNamespace(`root\wmi`),
			minRows:    3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, err := Query(ctx, tt.class, tt.properties)
			if tt.err {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tt.noData {
				assert.Empty(t, rows, "Expected no results")
				return
			}

			if tt.minRows > 0 {
				assert.GreaterOrEqual(t, len(rows), tt.minRows, "Expected minimum rows")
			}

		})
	}

}
