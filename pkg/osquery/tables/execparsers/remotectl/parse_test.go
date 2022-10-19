//go:build darwin
// +build darwin

package remotectl

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name        string
		input       []byte
		expectedErr bool
	}{
		{
			name:        "empty input",
			input:       []byte("\n"),
			expectedErr: false,
		},
		{
			name:        "dumpstate with single device in output",
			input:       readTestFile(t, path.Join("test-data", "single_device_dumpstate.txt")),
			expectedErr: false,
		},
		{
			name:        "dumpstate with multiple devices in output",
			input:       readTestFile(t, path.Join("test-data", "multiple_devices_dumpstate.txt")),
			expectedErr: false,
		},
		{
			name:        "malformed dumpstate output -- malformed top-level property",
			input:       readTestFile(t, path.Join("test-data", "malformed_dumpstate_at_top_level.txt")),
			expectedErr: true,
		},
		{
			name:        "malformed dumpstate output -- malformed item in Properties dict",
			input:       readTestFile(t, path.Join("test-data", "malformed_dumpstate_in_properties.txt")),
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := parseDumpstate(bytes.NewReader(tt.input))
			if tt.expectedErr {
				assert.Error(t, err)
				assert.Nil(t, result)
				return
			}

			require.NoError(t, err)

			resultMap := result.(map[string]map[string]interface{})

			for deviceName, deviceValues := range resultMap {
				validateItemInCommandOutput(t, deviceName, tt.input)
				// Confirm that we stripped "Found" from the front of the device name
				assert.False(t, strings.HasPrefix(deviceName, "Found"), fmt.Sprintf("device name not extracted correctly: got %s", deviceName))

				for topLevelKey, topLevelValue := range deviceValues {
					if topLevelKey == "Properties" {
						properties := topLevelValue.(map[string]interface{})
						for propertyKey, propertyValue := range properties {
							validateKeyValueInCommandOutput(t, propertyKey, propertyValue.(string), tt.input)
						}
					} else if topLevelKey == "Services" {
						services := topLevelValue.([]string)
						for _, service := range services {
							validateItemInCommandOutput(t, service, tt.input)
						}
					} else {
						validateKeyValueInCommandOutput(t, topLevelKey, topLevelValue.(string), tt.input)
					}
				}
			}
		})
	}
}

func readTestFile(t *testing.T, filepath string) []byte {
	b, err := os.ReadFile(filepath)
	require.NoError(t, err)
	return b
}

func validateKeyValueInCommandOutput(t *testing.T, key, val string, commandOutput []byte) {
	// First, confirm that the key and value both exists in the original output
	validateItemInCommandOutput(t, key, commandOutput)
	validateItemInCommandOutput(t, val, commandOutput)

	// Validate that the key and value were associated with each other
	regexFmt := `\Q%s\E.+\Q%s\E` // match key, then any delimiter, then value, on one line
	re := regexp.MustCompile(fmt.Sprintf(regexFmt, key, val))
	assert.True(t, re.Match(commandOutput), fmt.Sprintf("expected to see %s : %s in original command output but did not", key, val))
}

func validateItemInCommandOutput(t *testing.T, item string, commandOutput []byte) {
	assert.True(t, bytes.Contains(commandOutput, []byte(item)))
}
