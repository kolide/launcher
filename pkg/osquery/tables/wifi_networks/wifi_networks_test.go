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
			filename: "output.txt",
			expected: []map[string]string{
				{"fullkey": "DEFAULT/SSID", "parent": "DEFAULT", "key": "SSID", "value": "ddu23n104", "query": "", "ssid": "ddu23n104"},
				{"fullkey": "DEFAULT/BSSID", "parent": "DEFAULT", "key": "BSSID", "value": "A1-A2-A2-A2-A2-A2", "query": "", "ssid": "ddu23n104"},
				{"fullkey": "DEFAULT/dot11BssPhyType", "parent": "DEFAULT", "key": "dot11BssPhyType", "value": "7", "query": "", "ssid": "ddu23n104"},
				{"fullkey": "DEFAULT/timestamp", "parent": "DEFAULT", "key": "timestamp", "value": "610829842048", "query": "", "ssid": "ddu23n104"},
				{"fullkey": "DEFAULT/capabilityInformation", "parent": "DEFAULT", "key": "capabilityInformation", "value": "1073", "query": "", "ssid": "ddu23n104"},
				{"fullkey": "DEFAULT/ieOffset", "parent": "DEFAULT", "key": "ieOffset", "value": "1080", "query": "", "ssid": "ddu23n104"},
				{"fullkey": "DEFAULT/linkQuality", "parent": "DEFAULT", "key": "linkQuality", "value": "100", "query": "", "ssid": "ddu23n104"},
				{"fullkey": "DEFAULT/wlanRateSet", "parent": "DEFAULT", "key": "wlanRateSet", "value": "NativeWifi.Wlan+WlanRateSet", "query": "", "ssid": "ddu23n104"},
				{"fullkey": "DEFAULT/ieSize", "parent": "DEFAULT", "key": "ieSize", "value": "182", "query": "", "ssid": "ddu23n104"},
				{"fullkey": "DEFAULT/dot11Bssid", "parent": "DEFAULT", "key": "dot11Bssid", "value": "{128, 42, ..., 235...}", "query": "", "ssid": "ddu23n104"},
				{"fullkey": "DEFAULT/dot11BssType", "parent": "DEFAULT", "key": "dot11BssType", "value": "Infrastructure", "query": "", "ssid": "ddu23n104"},
				{"fullkey": "DEFAULT/beaconPeriod", "parent": "DEFAULT", "key": "beaconPeriod", "value": "0", "query": "", "ssid": "ddu23n104"},
				{"fullkey": "DEFAULT/chCenterFrequency", "parent": "DEFAULT", "key": "chCenterFrequency", "value": "2412000", "query": "", "ssid": "ddu23n104"},
				{"fullkey": "DEFAULT/hostTimestamp", "parent": "DEFAULT", "key": "hostTimestamp", "value": "132550441422028881", "query": "", "ssid": "ddu23n104"},
				{"fullkey": "DEFAULT/dot11Ssid", "parent": "DEFAULT", "key": "dot11Ssid", "value": "NativeWifi.Wlan+Dot11Ssid", "query": "", "ssid": "ddu23n104"},
				{"fullkey": "DEFAULT/phyId", "parent": "DEFAULT", "key": "phyId", "value": "2", "query": "", "ssid": "ddu23n104"},
				{"fullkey": "DEFAULT/rssi", "parent": "DEFAULT", "key": "rssi", "value": "-45", "query": "", "ssid": "ddu23n104"},
				{"fullkey": "DEFAULT/inRegDomain", "parent": "DEFAULT", "key": "inRegDomain", "value": "true", "query": "", "ssid": "ddu23n104"},
			},
		},
	}

	for _, tt := range tests {
		logger := log.NewNopLogger()
		table := WlanTable{
			logger: logger,
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
