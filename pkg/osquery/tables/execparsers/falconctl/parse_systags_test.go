package falconctl

import (
	"bytes"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSystags(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name        string
		input       []byte
		expected    any
		expectedErr bool
	}{
		{
			name:     "empty",
			expected: []string{},
		},
		{
			name:     "normal",
			input:    readTestFile(t, path.Join("test-data", "systags.txt")),
			expected: []string{"123", "12345678901234", "12345678901235", "12345678901236", "12345678901237", "12345678901238", "12345678901239", "12345678901230"},
		},
		{
			name:        "malformed",
			input:       []byte("123, this is malformed"),
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			actual, err := parseSystags(bytes.NewReader(tt.input))
			if tt.expectedErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expected, actual)
		})
	}
}

func readTestFile(t *testing.T, filepath string) []byte {
	b, err := os.ReadFile(filepath)
	require.NoError(t, err)
	return b
}
