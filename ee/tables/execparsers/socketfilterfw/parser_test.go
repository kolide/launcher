package socketfilterfw

import (
	"bytes"
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"
)

//go:embed test-data/apps.txt
var apps []byte

//go:embed test-data/data.txt
var data []byte

//go:embed test-data/empty.txt
var empty []byte

//go:embed test-data/malformed.txt
var malformed []byte

func TestParse(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name     string
		input    []byte
		expected []map[string]string
	}{
		{
			name:  "apps",
			input: apps,
			expected: []map[string]string{
				{
					"name":                       "replicatord",
					"allow_incoming_connections": "1",
				},
				{
					"name":                       "Pop Helper.app",
					"allow_incoming_connections": "0",
				},
				{
					"name":                       "Google Chrome",
					"allow_incoming_connections": "0",
				},
				{
					"name":                       "rtadvd",
					"allow_incoming_connections": "1",
				},
				{
					"name":                       "com.docker.backend",
					"allow_incoming_connections": "1",
				},
				{
					"name":                       "sshd-keygen-wrapper",
					"allow_incoming_connections": "1",
				},
			},
		},
		{
			name:  "data",
			input: data,
			expected: []map[string]string{
				{
					"global_state_enabled":            "1",
					"block_all_enabled":               "0",
					"allow_built-in_signed_enabled":   "1",
					"allow_downloaded_signed_enabled": "1",
					"stealth_enabled":                 "1",
					"logging_enabled":                 "1",
					"logging_option":                  "throttled",
				},
			},
		},
		{
			name:  "empty input",
			input: empty,
		},
		{
			name:  "malformed",
			input: malformed,
			expected: []map[string]string{
				{
					"global_state_enabled":            "0",
					"block_all_enabled":               "1",
					"allow_built-in_signed_enabled":   "0",
					"allow_downloaded_signed_enabled": "",
					"stealth_enabled":                 "0",
					"logging_enabled":                 "",
					"logging_option":                  "throttled",
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := New()
			result, err := p.Parse(bytes.NewReader(tt.input))
			require.NoError(t, err, "unexpected error parsing input")
			require.ElementsMatch(t, tt.expected, result)
		})
	}
}
