package socketfilterfw

import (
	"bytes"
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"
)

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
			name:  "empty input",
			input: empty,
		},
		{
			name:     "data",
			input:    data,
			expected: []map[string]string{
				{
				"global_state_enabled": "true",
				"block_all_enabled": "false",
				"allow_built-in_signed_enabled": "true",
				"allow_downloaded_signed_enabled": "true",
				"stealth_enabled": "false",
				"logging_enabled": "true",
				"logging_option": "throttled",
				},
			},
		},
		{
			name:  "malformed",
			input: malformed,
			expected: []map[string]string{
				{
				"global_state_enabled": "false",
				"block_all_enabled": "true",
				"allow_built-in_signed_enabled": "false",
				"allow_downloaded_signed_enabled": "true",
				"stealth_enabled": "false",
				"logging_enabled": "true",
				"logging_option": "throttled",
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
