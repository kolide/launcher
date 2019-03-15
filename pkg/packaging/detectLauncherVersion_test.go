package packaging

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// Various helpers are in packaging_test.go

func TestLauncherVersionDetection(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var err error

	p := &PackageOptions{}
	p.execCC = helperCommandContext

	err = p.detectLauncherVersion(ctx)
	require.NoError(t, err)

	require.Equal(t, "0.5.6-19-g17c8589", p.PackageVersion)
}

func TestFormatVersion(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		in  string
		out string
	}{
		{
			in:  "0.9.2-26-g6146437",
			out: "0.9.2.26",
		},
		{
			in:  "0.9.3-44",
			out: "0.9.3.44",
		},

		{
			in:  "0.9.5",
			out: "0.9.5.0",
		},
	}

	for _, tt := range tests {
		version, err := formatVersion(tt.in)
		require.NoError(t, err)
		require.Equal(t, tt.out, version)
	}

}
