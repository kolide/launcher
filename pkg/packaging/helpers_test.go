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

func TestFormatVersion(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		in       string
		platform PlatformFlavor
		out      string
	}{
		{
			in:       "0.9.2-26-g6146437",
			platform: Windows,
			out:      "0.9.2.26",
		},
		{
			in:       "0.9.3-44",
			platform: Windows,
			out:      "0.9.3.44",
		},

		{
			in:       "0.9.5",
			platform: Windows,
			out:      "0.9.5.0",
		},
		{
			in:       "0.9.2-26-g6146437",
			platform: Darwin,
			out:      "0.9.2",
		},
		{
			in:       "0.9.3-44",
			platform: Darwin,
			out:      "0.9.3",
		},
		{
			in:       "v0.9.5",
			platform: Darwin,
			out:      "0.9.5",
		},
		{
			in:       "v10.8.2-1002df",
			platform: Darwin,
			out:      "10.8.2",
		},
		{
			in:       "0.9.2-26-g6146437",
			platform: Linux,
			out:      "0.9.2-26-g6146437",
		},
	}

	for _, tt := range tests {
		version, err := formatVersion(tt.in, tt.platform)
		require.NoError(t, err)
		require.Equal(t, tt.out, version)
	}
}
