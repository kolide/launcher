package wlan

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
		// {
		// 	filename: "results2.txt",
		// 	expected: []map[string]string{},
		// },
		// {
		// 	filename: "results.txt",
		// 	expected: []map[string]string{},
		// },
		{
			filename: "results3.txt",
			expected: []map[string]string{
				{
					"name":                       "ddu23n104",
					"authentication":             "WPA2-Personal",
					"signal_strength_percentage": "92",
					"bssid":                      "80:2a:9c:eb:bb:65",
					"radio_type":                 "802.11n",
				},
				{
					"name":                       "",
					"authentication":             "WPA2-Personal",
					"signal_strength_percentage": "92",
					"bssid":                      "82:2a:a8:eb:bb:65",
					"radio_type":                 "802.11n",
				},
				{
					"name":                       "GMG_DB_315",
					"authentication":             "WPA2-Personal",
					"signal_strength_percentage": "34",
					"bssid":                      "08:ea:88:84:cf:6c",
					"radio_type":                 "802.11n",
				},
				{
					"name":                       "MySpectrumWiFi90-2G",
					"authentication":             "WPA2-Personal",
					"signal_strength_percentage": "0",
					"bssid":                      "7c:db:98:b3:e0:8e",
					"radio_type":                 "802.11ac",
				},
			},
		},
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
			if len(results) > 0 {
				for _, r := range results {
					jsonString, err := json.Marshal(r)
					require.NoError(t, err, "marshalling json: %s", tt.filename)
					fmt.Println(string(jsonString))
				}
			}

			for _, r := range tt.expected {
				jsonString, err := json.Marshal(r)
				require.NoError(t, err, "marshalling json: %s", tt.filename)
				fmt.Printf("\033[35m%s\033[0m\n", jsonString)
			}

			require.ElementsMatch(t, tt.expected, results)
		})

	}
}
