//go:build darwin
// +build darwin

package mdmclient

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func TestTransformOutput(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		in           string
		expectedRows int
	}{
		{
			in:           "QueryDeviceInformation.output",
			expectedRows: 1659,
		},
		{
			in:           "QueryDeviceInformation_WithHeader.output",
			expectedRows: 96,
		},
		{
			in:           "QueryDeviceInformation_NullAgentResponse.output",
			expectedRows: 60,
		},
		{
			in:           "QueryInstalledProfiles.output",
			expectedRows: 32,
		},
		{
			in:           "QuerySecurityInfo.output",
			expectedRows: 219,
		},
	}

	table := Table{slogger: multislogger.NewNopLogger()}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			input, err := os.ReadFile(filepath.Join("testdata", tt.in))
			require.NoError(t, err, "read input file")

			output, err := table.flattenOutput(t.Context(), "", input)
			require.NoError(t, err, "flatten")
			require.Equal(t, tt.expectedRows, len(output))

		})
	}
}
