package dataflatten

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_duplicateKeyFunc(t *testing.T) {
	t.Parallel()

	// Test nmcli output
	rawData := []byte(`                                       
SSID:                                   VIDEOTRON2255
MODE:                                   Infra
CHAN:                                   11
RATE:                                   54 Mbit/s
SIGNAL:                                 70
SECURITY:                               WPA1 WPA2
	`)

	dataFunc := StringDelimitedFunc(":", DuplicateKeys)

	rows, err := dataFunc(rawData)
	require.NoError(t, err, "did not expect error flattening KV data")

	for _, r := range rows {
		require.Equal(t, 2, len(r.Path), "unexpected path length")
		switch r.Path[1] {
		case "SSID":
			require.Equal(t, r.Value, "VIDEOTRON2255")
		case "MODE":
			require.Equal(t, r.Value, "Infra")
		case "CHAN":
			require.Equal(t, r.Value, "11")
		case "RATE":
			require.Equal(t, r.Value, "54 Mbit/s")
		case "SIGNAL":
			require.Equal(t, r.Value, "70")
		case "SECURITY":
			require.Equal(t, r.Value, "WPA1 WPA2")
		default:
			t.Errorf("unexpected path: %+v", r.Path)
			t.FailNow()
		}
	}
	require.Equal(t, 6, len(rows), "unexpected number of rows")
}
