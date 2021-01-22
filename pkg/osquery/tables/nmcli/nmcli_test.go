package nmcli

import (
	"context"
	// "encoding/json"
	// "fmt"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/stretchr/testify/require"
)

func TestGenerate(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		filename     string
		expectedRows []map[string]string
	}{
		{
			filename: "multiline_multiple_networks.txt",
			expectedRows: []map[string]string{
				{"bssid": "AA:11:33:0F:BB:22", "ssid": "JBW-2G", "channel": "6", "rate": "405 Mbit/s", "signal": "100", "security": "WPA2"},
				{"bssid": "AA:11:33:0F:BB:22", "ssid": "example.example.com", "channel": "36", "rate": "270 Mbit/s", "signal": "94", "security": "WPA2"},
				{"bssid": "AA:11:33:0F:BB:22", "ssid": "JBW-5G", "channel": "149", "rate": "540 Mbit/s", "signal": "94", "security": "WPA2"},
				{"bssid": "AA:11:33:0F:BB:22", "ssid": "", "channel": "11", "rate": "130 Mbit/s", "signal": "85", "security": "WPA2"},
				{"bssid": "AA:11:33:0F:BB:22", "ssid": "Jwc5834", "channel": "11", "rate": "130 Mbit/s", "signal": "82", "security": "WPA2"},
				{"bssid": "AA:11:33:0F:BB:22", "ssid": "example.example.com", "channel": "11", "rate": "130 Mbit/s", "signal": "82", "security": "WPA2"},
				{"bssid": "AA:11:33:0F:BB:22", "ssid": "Call 555-555-2903 for Tech Supp", "channel": "11", "rate": "130 Mbit/s", "signal": "82", "security": "WPA2"},
				{"bssid": "AA:11:33:0F:BB:22", "ssid": "example.com 555-2903", "channel": "11", "rate": "130 Mbit/s", "signal": "80", "security": "WPA2"},
				{"bssid": "AA:11:33:0F:BB:22", "ssid": "TP-LINK_4F38", "channel": "10", "rate": "130 Mbit/s", "signal": "32", "security": "WPA2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			logger := log.NewNopLogger()
			table := Table{
				logger: logger,
				parser: newParser(logger),
				getBytes: func(ctx context.Context, fields []string) ([]byte, error) {
					input, err := ioutil.ReadFile(filepath.Join("testdata", tt.filename))
					require.NoError(t, err, "reading input file")
					return input, nil
				},
			}
			ctx := context.TODO()
			qCon := tablehelpers.MockQueryContext(map[string][]string{})

			actual, err := table.generate(ctx, qCon)
			// for _, r := range actual {
			// 	jsonString, err := json.Marshal(r)
			// 	require.NoError(t, err, "marshalling json: %s", tt.filename)
			// 	fmt.Println(string(jsonString))
			// }

			// for _, r := range tt.expectedRows {
			// 	jsonString, err := json.Marshal(r)
			// 	require.NoError(t, err, "marshalling json: %s", tt.filename)
			// 	fmt.Printf("\033[35m%s\033[0m\n", jsonString)
			// }

			require.NoError(t, err, "generating results from %s", tt.filename)
			require.ElementsMatch(t, tt.expectedRows, actual)
		})
	}
}
