//go:build darwin || linux
// +build darwin linux

package key_value

import (
	_ "embed"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed test-data/nmcli.txt
var nmcliTestData string

func TestParseKeyValue(t *testing.T) {
	t.Run("simple key-value pairs", func(t *testing.T) {
		p := New()
		input := "key1=value1\nkey2=value2"
		reader := strings.NewReader(input)
		result, err := p.Parse(reader)
		require.NoError(t, err)

		expected := map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		}
		assert.Equal(t, expected, result)
	})

	t.Run("with comments and empty lines", func(t *testing.T) {
		p := New()
		input := `
			# This is a comment
			key1=value1

			// Another comment
			key2 = value2
		`
		reader := strings.NewReader(input)
		result, err := p.Parse(reader)
		require.NoError(t, err)

		expected := map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		}
		assert.Equal(t, expected, result)
	})

	t.Run("custom delimiter", func(t *testing.T) {
		p := NewWithDelimiter(":")
		input := "key1:value1\nkey2: value2 "
		reader := strings.NewReader(input)
		result, err := p.Parse(reader)
		require.NoError(t, err)

		expected := map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		}
		assert.Equal(t, expected, result)
	})

	t.Run("quoted values", func(t *testing.T) {
		p := New()
		input := `
			key1="value1"
			key2='value2'
			key3="value with spaces"
			key4='another value with spaces'
			key5=unquoted
			`
		reader := strings.NewReader(input)
		result, err := p.Parse(reader)
		require.NoError(t, err)

		expected := map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
			"key3": "value with spaces",
			"key4": "another value with spaces",
			"key5": "unquoted",
		}
		assert.Equal(t, expected, result)
	})

	t.Run("duplicate keys", func(t *testing.T) {
		p := New()
		input := `
		key1=value1
		key1=value2
		key2=valueA
		key1=value3
		`
		reader := strings.NewReader(input)
		result, err := p.Parse(reader)
		require.NoError(t, err)

		expected := map[string]interface{}{
			"key1": []interface{}{"value1", "value2", "value3"},
			"key2": "valueA",
		}
		assert.Equal(t, expected, result)
	})

	t.Run("nested keys", func(t *testing.T) {
		p := New()
		input := `
		section.key1=value1
		section.key2=value2
		other.keyA=valueA
		`
		reader := strings.NewReader(input)
		result, err := p.Parse(reader)
		require.NoError(t, err)

		expected := map[string]interface{}{
			"section": map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
			"other": map[string]interface{}{
				"keyA": "valueA",
			},
		}
		assert.Equal(t, expected, result)
	})

	t.Run("nested keys with duplicates", func(t *testing.T) {
		p := New()
		input := `
		section.key1=value1
		section.key1=value2
		section.key2=valueA
		`
		reader := strings.NewReader(input)
		result, err := p.Parse(reader)
		require.NoError(t, err)

		expected := map[string]interface{}{
			"section": map[string]interface{}{
				"key1": []interface{}{"value1", "value2"},
				"key2": "valueA",
			},
		}
		assert.Equal(t, expected, result)
	})

	t.Run("malformed lines", func(t *testing.T) {
		p := New()
		input := `
		key1=value1
		malformed_line
		key2 = value2
		key3=
		=value4
		`
		reader := strings.NewReader(input)
		result, err := p.Parse(reader)
		require.NoError(t, err)

		expected := map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
			"key3": "",       // Value is empty string
			"":     "value4", // Key is empty string
		}
		// Note: The current parser implementation will create an empty key if the line starts with a delimiter
		// and an empty value if the line ends with a delimiter.
		// If this behavior is not desired, the parser logic should be adjusted.
		// For now, testing the current behavior.
		assert.Equal(t, expected, result)
	})

	t.Run("empty input", func(t *testing.T) {
		p := New()
		input := ""
		reader := strings.NewReader(input)
		result, err := p.Parse(reader)
		require.NoError(t, err)
		assert.Equal(t, map[string]interface{}{}, result)
	})

	t.Run("nmcli data", func(t *testing.T) {
		p := NewWithDelimiter(":") // nmcli output uses ":" as delimiter)

		reader := strings.NewReader(string(nmcliTestData))
		result, err := p.Parse(reader)
		require.NoError(t, err)
		require.NotNil(t, result)

		parsedResult, ok := result.(map[string]interface{})
		require.True(t, ok, "Parsed result is not a map[string]interface{}")

		const expectedEntries = 39 // AP[1] to AP[39]

		// Helper function to get an array field and check its length
		getArrayField := func(key string) []interface{} {
			assert.Contains(t, parsedResult, key)
			field, ok := parsedResult[key].([]interface{})
			require.True(t, ok, "%s field is not an array", key)
			assert.Len(t, field, expectedEntries, "Length of %s array is not %d", key, expectedEntries)
			return field
		}

		// NAME
		nameField := getArrayField("NAME")
		assert.Equal(t, "AP[1]", nameField[0])
		assert.Equal(t, "AP[2]", nameField[1])
		assert.Equal(t, "AP[15]", nameField[14]) // Middle entry
		assert.Equal(t, "AP[39]", nameField[38]) // Last entry

		// SSID
		ssidField := getArrayField("SSID")
		assert.Equal(t, "JAMGUEST", ssidField[0])                          // AP[1]
		assert.Equal(t, "JAM5G", ssidField[1])                             // AP[2]
		assert.Equal(t, "--", ssidField[4])                                // AP[5] has SSID "--"
		assert.Equal(t, "[range]_E30AJT7113353H", ssidField[14])           // AP[15]
		assert.Equal(t, "DIRECT-A7-HP DeskJet 2800 series", ssidField[38]) // AP[39]

		// SSID-HEX
		ssidHexField := getArrayField("SSID-HEX")
		assert.Equal(t, "4A414D4755455354", ssidHexField[0])                                                  // AP[1]
		assert.Equal(t, "--", ssidHexField[4])                                                                // AP[5]
		assert.Equal(t, "5B72616E67655D5F453330414A543731313333353348", ssidHexField[14])                     // AP[15]
		assert.Equal(t, "4449524543542D41372D4850204465736B4A6574203238303020736572696573", ssidHexField[38]) // AP[39]

		// BSSID
		bssidField := getArrayField("BSSID")
		assert.Equal(t, "D8:50:E6:94:7A:28", bssidField[0])  // AP[1]
		assert.Equal(t, "72:13:01:84:A7:FC", bssidField[4])  // AP[5]
		assert.Equal(t, "50:FD:D5:1A:15:C2", bssidField[14]) // AP[15]
		assert.Equal(t, "6C:0B:5E:C5:61:A8", bssidField[38]) // AP[39]

		// MODE
		modeField := getArrayField("MODE")
		assert.Equal(t, "Infra", modeField[0])  // AP[1]
		assert.Equal(t, "Infra", modeField[14]) // AP[15]
		assert.Equal(t, "Infra", modeField[38]) // AP[39] (all are Infra)

		// CHAN
		chanField := getArrayField("CHAN")
		assert.Equal(t, "9", chanField[0])  // AP[1]
		assert.Equal(t, "1", chanField[14]) // AP[15]
		assert.Equal(t, "6", chanField[38]) // AP[39]

		// FREQ
		freqField := getArrayField("FREQ")
		assert.Equal(t, "2452 MHz", freqField[0])  // AP[1]
		assert.Equal(t, "2412 MHz", freqField[14]) // AP[15]
		assert.Equal(t, "2437 MHz", freqField[38]) // AP[39]

		// RATE
		rateField := getArrayField("RATE")
		assert.Equal(t, "195 Mbit/s", rateField[0])  // AP[1]
		assert.Equal(t, "135 Mbit/s", rateField[14]) // AP[15]
		assert.Equal(t, "65 Mbit/s", rateField[38])  // AP[39]

		// SIGNAL
		signalField := getArrayField("SIGNAL")
		assert.Equal(t, "87", signalField[0])  // AP[1]
		assert.Equal(t, "29", signalField[14]) // AP[15]
		assert.Equal(t, "14", signalField[38]) // AP[39]

		// BARS
		barsField := getArrayField("BARS")
		assert.Equal(t, "▂▄▆█", barsField[0])  // AP[1]
		assert.Equal(t, "▂___", barsField[14]) // AP[15]
		assert.Equal(t, "▂___", barsField[38]) // AP[39]

		// SECURITY
		securityField := getArrayField("SECURITY")
		assert.Equal(t, "WPA2", securityField[0])         // AP[1]
		assert.Equal(t, "--", securityField[7])           // AP[8] has SECURITY "--"
		assert.Equal(t, "WPA2", securityField[14])        // AP[15]
		assert.Equal(t, "WPA2 802.1X", securityField[24]) // AP[25]
		assert.Equal(t, "WPA2", securityField[38])        // AP[39]

		// WPA-FLAGS
		wpaFlagsField := getArrayField("WPA-FLAGS")
		assert.Equal(t, "(none)", wpaFlagsField[0])                              // AP[1]
		assert.Equal(t, "pair_tkip pair_ccmp group_tkip psk", wpaFlagsField[23]) // AP[24]
		assert.Equal(t, "(none)", wpaFlagsField[38])                             // AP[39]

		// RSN-FLAGS
		rsnFlagsField := getArrayField("RSN-FLAGS")
		assert.Equal(t, "pair_ccmp group_ccmp psk", rsnFlagsField[0])      // AP[1]
		assert.Equal(t, "(none)", rsnFlagsField[23])                       // AP[24]
		assert.Equal(t, "pair_ccmp group_ccmp psk sae", rsnFlagsField[31]) // AP[32]
		assert.Equal(t, "pair_ccmp group_ccmp psk", rsnFlagsField[38])     // AP[39]

		// DEVICE
		deviceField := getArrayField("DEVICE")
		assert.Equal(t, "wlo1", deviceField[0])  // AP[1]
		assert.Equal(t, "wlo1", deviceField[14]) // AP[15]
		assert.Equal(t, "wlo1", deviceField[38]) // AP[39] (all are wlo1)

		// ACTIVE
		activeField := getArrayField("ACTIVE")
		assert.Equal(t, "no", activeField[0])  // AP[1]
		assert.Equal(t, "yes", activeField[1]) // AP[2]
		assert.Equal(t, "no", activeField[14]) // AP[15]
		assert.Equal(t, "no", activeField[38]) // AP[39]

		// IN-USE
		inuseField := getArrayField("IN-USE")
		assert.Equal(t, "", inuseField[0])  // AP[1] (empty)
		assert.Equal(t, "*", inuseField[1]) // AP[2]
		assert.Equal(t, "", inuseField[14]) // AP[15] (empty)
		assert.Equal(t, "", inuseField[38]) // AP[39] (empty)

		// DBUS-PATH
		dbusPathField := getArrayField("DBUS-PATH")
		assert.Equal(t, "/org/freedesktop/NetworkManager/AccessPoint/4", dbusPathField[0])   // AP[1]
		assert.Equal(t, "/org/freedesktop/NetworkManager/AccessPoint/22", dbusPathField[14]) // AP[15]
		assert.Equal(t, "/org/freedesktop/NetworkManager/AccessPoint/45", dbusPathField[38]) // AP[39]
	})

	t.Run("set delimiter method", func(t *testing.T) {
		p := New() // Default is "="
		p.SetDelimiter(":")
		input := "key1:value1\nkey2:value2"
		reader := strings.NewReader(input)
		result, err := p.Parse(reader)
		require.NoError(t, err)

		expected := map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		}
		assert.Equal(t, expected, result)
	})

	t.Run("set delimiter method", func(t *testing.T) {
		p := New() // Default is "="
		p.SetDelimiter(":")
		input := "key1:value1\nkey2:value2"
		reader := strings.NewReader(input)
		result, err := p.Parse(reader)
		require.NoError(t, err)

		expected := map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		}
		assert.Equal(t, expected, result)
	})

	t.Run("nested key conflict with existing non-map value", func(t *testing.T) {
		p := New()
		input := `
		parent=iamastring
		parent.child=value
		`
		reader := strings.NewReader(input)
		result, err := p.Parse(reader)
		require.NoError(t, err)

		expected := map[string]interface{}{
			"parent": map[string]interface{}{
				"child": "value",
			},
		}
		assert.Equal(t, expected, result)
	})

	t.Run("deeply nested keys", func(t *testing.T) {
		p := New()
		input := "a.b.c.d.e=deep_value"
		reader := strings.NewReader(input)
		result, err := p.Parse(reader)
		require.NoError(t, err)

		expected := map[string]interface{}{
			"a": map[string]interface{}{
				"b": map[string]interface{}{
					"c": map[string]interface{}{
						"d": map[string]interface{}{
							"e": "deep_value",
						},
					},
				},
			},
		}
		assert.Equal(t, expected, result)
	})

	t.Run("keys with spaces (if delimiter allows)", func(t *testing.T) {
		p := New() // Delimiter is "="
		input := "key with spaces = value with spaces"
		reader := strings.NewReader(input)
		result, err := p.Parse(reader)
		require.NoError(t, err)

		expected := map[string]interface{}{
			"key with spaces": "value with spaces",
		}
		assert.Equal(t, expected, result)
	})

	t.Run("values with delimiter character", func(t *testing.T) {
		p := New()
		input := `key1=value1=with=equals
		key2="value2=with=equals"
		`
		reader := strings.NewReader(input)
		result, err := p.Parse(reader)
		require.NoError(t, err)

		expected := map[string]interface{}{
			"key1": "value1=with=equals", // SplitN ensures this
			"key2": "value2=with=equals", // Quotes are removed
		}
		assert.Equal(t, expected, result)
	})
}
