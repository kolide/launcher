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
	t.Parallel()
	t.Run("simple key-value pairs", func(t *testing.T) {
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
		p := New()
		input := ""
		reader := strings.NewReader(input)
		result, err := p.Parse(reader)
		require.NoError(t, err)
		assert.Equal(t, map[string]interface{}{}, result)
	})

	t.Run("nmcli data", func(t *testing.T) {
		t.Parallel()
		p := NewWithDelimiter(":") // nmcli output uses ":" as delimiter)

		reader := strings.NewReader(nmcliTestData)
		result, err := p.Parse(reader)
		require.NoError(t, err)
		require.NotNil(t, result)

		parsedResult, ok := result.(map[string]interface{})
		require.True(t, ok, "Parsed result is not a map[string]interface{}")

		const expectedEntries = 7 // AP[1] to AP[7]

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
		assert.Equal(t, "AP[3]", nameField[2]) // Middle entry
		assert.Equal(t, "AP[7]", nameField[6]) // Last entry

		// SSID
		ssidField := getArrayField("SSID")
		assert.Equal(t, "TESTSSID", ssidField[0])    // AP[1]
		assert.Equal(t, "TESTSSID-5g", ssidField[1]) // AP[2]
		assert.Equal(t, "--", ssidField[4])          // AP[5] has SSID "--"
		assert.Equal(t, "--", ssidField[6])          // AP[7]
		assert.Equal(t, "TESTSSID-5G", ssidField[2]) // AP[3]

		// SSID-HEX
		ssidHexField := getArrayField("SSID-HEX")
		assert.Equal(t, "4A414D4755455354", ssidHexField[0])       // AP[1]
		assert.Equal(t, "--", ssidHexField[4])                     // AP[5]
		assert.Equal(t, "--", ssidHexField[6])                     // AP[7]
		assert.Equal(t, "4A414D47554553542D3547", ssidHexField[2]) // AP[3]

		// BSSID
		bssidField := getArrayField("BSSID")
		assert.Equal(t, "1A:23:B7:DE:77:2C", bssidField[0]) // AP[1]
		assert.Equal(t, "72:13:01:84:A7:FC", bssidField[4]) // AP[5]
		assert.Equal(t, "72:13:01:84:A7:FE", bssidField[6]) // AP[7]
		assert.Equal(t, "1A:23:B7:DE:7A:2D", bssidField[2]) // AP[3]

		// MODE
		modeField := getArrayField("MODE")
		assert.Equal(t, "Infra", modeField[0]) // AP[1]
		assert.Equal(t, "Infra", modeField[6]) // AP[7]
		assert.Equal(t, "Infra", modeField[2]) // AP[3] (all are Infra)

		// CHAN
		chanField := getArrayField("CHAN")
		assert.Equal(t, "9", chanField[0])  // AP[1]
		assert.Equal(t, "6", chanField[6])  // AP[7]
		assert.Equal(t, "40", chanField[2]) // AP[3]

		// FREQ
		freqField := getArrayField("FREQ")
		assert.Equal(t, "2452 MHz", freqField[0]) // AP[1]
		assert.Equal(t, "2437 MHz", freqField[6]) // AP[7]
		assert.Equal(t, "5200 MHz", freqField[2]) // AP[3]

		// RATE
		rateField := getArrayField("RATE")
		assert.Equal(t, "195 Mbit/s", rateField[0]) // AP[1]
		assert.Equal(t, "540 Mbit/s", rateField[6]) // AP[7]
		assert.Equal(t, "405 Mbit/s", rateField[2]) // AP[3]

		// SIGNAL
		signalField := getArrayField("SIGNAL")
		assert.Equal(t, "87", signalField[0]) // AP[1]
		assert.Equal(t, "42", signalField[6]) // AP[7]
		assert.Equal(t, "72", signalField[2]) // AP[3]

		// BARS
		barsField := getArrayField("BARS")
		assert.Equal(t, "▂▄▆█", barsField[0]) // AP[1]
		assert.Equal(t, "▂▄__", barsField[6]) // AP[7]
		assert.Equal(t, "▂▄▆_", barsField[2]) // AP[3]

		// SECURITY
		securityField := getArrayField("SECURITY")
		assert.Equal(t, "WPA2", securityField[0])        // AP[1]
		assert.Equal(t, "WPA2 802.1X", securityField[6]) // AP[7]
		assert.Equal(t, "WPA2", securityField[2])        // AP[3]

		// WPA-FLAGS
		wpaFlagsField := getArrayField("WPA-FLAGS")
		assert.Equal(t, "(none)", wpaFlagsField[0]) // AP[1]

		assert.Equal(t, "(none)", wpaFlagsField[2]) // AP[3]

		// RSN-FLAGS
		rsnFlagsField := getArrayField("RSN-FLAGS")
		assert.Equal(t, "pair_ccmp group_ccmp psk", rsnFlagsField[0]) // AP[1]
		assert.Equal(t, "pair_ccmp group_ccmp psk", rsnFlagsField[2]) // AP[3]

		// DEVICE
		deviceField := getArrayField("DEVICE")
		assert.Equal(t, "wlo1", deviceField[0]) // AP[1]
		assert.Equal(t, "wlo1", deviceField[6]) // AP[7]
		assert.Equal(t, "wlo1", deviceField[2]) // AP[3] (all are wlo1)

		// ACTIVE
		activeField := getArrayField("ACTIVE")
		assert.Equal(t, "no", activeField[0])  // AP[1]
		assert.Equal(t, "yes", activeField[1]) // AP[2]
		assert.Equal(t, "no", activeField[6])  // AP[7]
		assert.Equal(t, "no", activeField[2])  // AP[3]

		// IN-USE
		inuseField := getArrayField("IN-USE")
		assert.Equal(t, "", inuseField[0])  // AP[1] (empty)
		assert.Equal(t, "*", inuseField[1]) // AP[2]
		assert.Equal(t, "", inuseField[6])  // AP[7] (empty)
		assert.Equal(t, "", inuseField[2])  // AP[3] (empty)

		// DBUS-PATH
		dbusPathField := getArrayField("DBUS-PATH")
		assert.Equal(t, "/org/freedesktop/NetworkManager/AccessPoint/4", dbusPathField[0]) // AP[1]
		assert.Equal(t, "/org/freedesktop/NetworkManager/AccessPoint/9", dbusPathField[6]) // AP[7]
		assert.Equal(t, "/org/freedesktop/NetworkManager/AccessPoint/3", dbusPathField[2]) // AP[3]
	})

	t.Run("set delimiter method", func(t *testing.T) {
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
		t.Parallel()
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
