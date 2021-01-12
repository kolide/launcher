package wifi_networks

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/stretchr/testify/require"
)

func TestTableGenerate(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		filename string
		expected []map[string]string
	}{
		{
			filename: "blank.txt",
			expected: []map[string]string{},
		},
		{
			filename: "results_pwsh.txt",
			expected: []map[string]string{
				{
					"name":                       "",
					"rssi":                       "-43",
					"bssid":                      "82:2B:A3:EB:93:65",
					"signal_strength_percentage": "90",
				},
				{
					"name":                       "ddu23n104",
					"rssi":                       "-43",
					"bssid":                      "88:2B:A3:EB:93:65",
					"signal_strength_percentage": "90",
				},
			},
		},
		{
			filename: "extra_blank_lines.txt",
			expected: []map[string]string{
				{
					"name":                       "",
					"rssi":                       "-43",
					"bssid":                      "82:2B:A3:EB:93:65",
					"signal_strength_percentage": "90",
				},
				{
					"name":                       "ddu23n104",
					"rssi":                       "-43",
					"bssid":                      "88:2B:A3:EB:93:65",
					"signal_strength_percentage": "90",
				},
			},
		},
	}

	for _, tt := range tests {
		logger := log.NewNopLogger()
		table := WlanTable{
			logger: logger,
			parser: buildParserFull(logger),
			getBytes: func(ctx context.Context, buf *bytes.Buffer) error {
				f, err := os.Open(filepath.Join("testdata", tt.filename))
				require.NoError(t, err, "opening file %s", tt.filename)
				defer f.Close()

				_, err = buf.ReadFrom(f)
				require.NoError(t, err, "read file %s", tt.filename)

				return nil
			},
		}

		t.Run(tt.filename, func(t *testing.T) {
			ctx := context.TODO()
			qCon := tablehelpers.MockQueryContext(map[string][]string{})

			results, err := table.generate(ctx, qCon)
			require.NoError(t, err, "generating results from %s", tt.filename)
			require.ElementsMatch(t, tt.expected, results)
		})
	}
}
