package wlan

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
			filename: "multiple_results.txt",
			expected: []map[string]string{
				{
					"name":                       "ddu23n104",
					"authentication":             "WPA2-Personal",
					"signal_strength_percentage": "92",
					"rssi":                       "-54",
					"bssid":                      "80:2a:9c:eb:bb:65",
					"radio_type":                 "802.11n",
					"channel":                    "1",
				},
				{
					"name":                       "",
					"authentication":             "WPA2-Personal",
					"signal_strength_percentage": "92",
					"rssi":                       "-54",
					"bssid":                      "82:2a:a8:eb:bb:65",
					"radio_type":                 "802.11n",
					"channel":                    "1",
				},
				{
					"name":                       "GMG_DB_315",
					"authentication":             "WPA2-Personal",
					"signal_strength_percentage": "34",
					"rssi":                       "-83",
					"bssid":                      "08:ea:88:84:cf:6c",
					"radio_type":                 "802.11n",
					"channel":                    "7",
				},
				{
					"name":                       "MySpectrumWiFi90-2G",
					"authentication":             "WPA2-Personal",
					"signal_strength_percentage": "0",
					"rssi":                       "-100",
					"bssid":                      "7c:db:98:b3:e0:8e",
					"radio_type":                 "802.11ac",
					"channel":                    "11",
				},
			},
		},
		// {
		// 	filename: "resultsps.txt",
		// 	expected: []map[string]string{
		// 		{
		// 			"name":  "",
		// 			"rssi":  "-43",
		// 			"bssid": "82:2A:A8:EB:93:65",
		// 		},
		// 		{
		// 			"name":  "ddu23n104",
		// 			"rssi":  "-43",
		// 			"bssid": "82:2A:A8:EB:93:65",
		// 		},
		// 	},
		// },
	}

	for _, tt := range tests {
		logger := log.NewNopLogger()
		table := WlanTable{
			logger: logger,
			parser: buildParser(logger),
			getBytes: func(ctx context.Context, buf *bytes.Buffer) error {
				f, err := os.Open(filepath.Join("testdata", tt.filename))
				defer f.Close()

				require.NoError(t, err, "opening file %s", tt.filename)
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
