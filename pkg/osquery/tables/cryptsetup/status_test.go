package cryptsetup

import (
	"io/ioutil"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseStatus(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		infile  string
		len     int
		status  string
		mounted bool
	}{
		{
			infile:  "status-active-luks2.txt",
			status:  "active",
			mounted: true,
		},
		{
			infile:  "status-active-mounted.txt",
			status:  "active",
			mounted: true,
		},
		{
			infile: "status-active-umounted.txt",
			status: "active",
		},
		{
			infile:  "status-active.txt",
			status:  "active",
			mounted: true,
		},
		{
			infile: "status-error.txt",
			status: "not_found",
		},
		{
			infile: "status-inactive.txt",
			status: "inactive",
		},
		{
			infile: "status-unactive.txt",
			status: "inactive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.infile, func(t *testing.T) {
			input, err := ioutil.ReadFile(filepath.Join("testdata", tt.infile))
			require.NoError(t, err, "read input file")

			data, err := parseStatus(input)
			require.NoError(t, err, "parseStatus")

			require.Equal(t, tt.status, data["status"], "status")
			require.Equal(t, strconv.FormatBool(tt.mounted), data["mounted"], "mounted")
		})
	}
}
