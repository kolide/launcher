// +build !windows
// (skip building windows, since the newline replacement doesn't work there)

package remotectl

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
			in:           "imacpro-w-iphone.output",
			expectedRows: 4900,
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
