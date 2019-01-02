package packaging

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeHostname(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		in  string
		out string
	}{
		{
			in:  "hostname",
			out: "hostname",
		},
		{
			in:  "hello:colon",
			out: "hello-colon",
		},
	}

	for _, tt := range tests {
		require.Equal(t, tt.out, sanitizeHostname(tt.in))
	}
}
