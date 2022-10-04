package falconctl

import (
	"bytes"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseOptions(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name        string
		input       []byte
		expected    any
		expectedErr bool
	}{
		{
			name:     "empty",
			expected: map[string]any{},
		},
		{
			name:  "normal",
			input: readTestFile(t, path.Join("test-data", "options.txt")),
			expected: map[string]interface{}{
				"aid":            "is not set",
				"aph":            "is not set",
				"app":            "is not set",
				"cid":            "ac917ab****************************",
				"feature":        "is not set",
				"metadata-query": "enable",
				"rfm-reason":     "is not set",
				"rfm-state":      "is not set",
				"version ":       "6.38.13501.0"},
		},
		{
			name:        "cid not set",
			input:       readTestFile(t, path.Join("test-data", "cid-error.txt")),
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			actual, err := parseOptions(bytes.NewReader(tt.input))
			if tt.expectedErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expected, actual)
		})
	}

}
