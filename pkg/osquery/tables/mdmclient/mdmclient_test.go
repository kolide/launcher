// +build !windows
// (skip building windows, since the newline replacement doesn't work there)

package mdmclient

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/go-kit/kit/log"
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
			in:           "QueryInstalledProfiles.output",
			expectedRows: 30,
		},
		{
			in:           "QuerySecurityInfo.output",
			expectedRows: 219,
		},
	}

	table := Table{logger: log.NewNopLogger()}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			input, err := ioutil.ReadFile(filepath.Join("testdata", tt.in))
			require.NoError(t, err, "read input file")

			output, err := table.flattenOutput("", input)
			require.NoError(t, err, "flatten")
			require.Equal(t, tt.expectedRows, len(output))

		})
	}
}
