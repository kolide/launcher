//go:build darwin || linux
// +build darwin linux

package mapxml

import (
	"bytes"
	_ "embed"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed test-data/single-install.txt
var singleInstallData string

//go:embed test-data/multiple-installs.txt
var multipleInstallsData string

func TestParse(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name               string
		input              []byte
		expectedItemCount  int
		expectedAttributes map[string]string
		expectedErr        bool
	}{
		{
			name:              "empty input",
			input:             []byte(""),
			expectedItemCount: 0,
			expectedErr:       true,
		},
		{
			name:              "single install data",
			input:             []byte(singleInstallData),
			expectedItemCount: 1,
			expectedAttributes: map[string]string{
				"attrPath":           "0",
				"maxComparedVersion": "2.49.0",
				"name":               "git-2.49.0",
				"outputName":         "",
				"pname":              "git",
				"system":             "aarch64-darwin",
				"version":            "2.49.0",
				"versionDiff":        "=",
			},
			expectedErr: false,
		},
		{
			name:              "multiple installs data",
			input:             []byte(multipleInstallsData),
			expectedItemCount: 3,
			expectedErr:       false,
		},
		{
			name:              "malformed XML",
			input:             []byte("<items><item>malformed"),
			expectedItemCount: 0,
			expectedErr:       true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := New()
			result, err := p.Parse(bytes.NewReader(tt.input))

			if tt.expectedErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Check the structure of the parsed data
			resultMap, ok := result.(map[string]interface{})
			require.True(t, ok, "Result should be a map[string]interface{}")

			// Check if we have items
			items, ok := resultMap["items"]
			require.True(t, ok, "Result should contain 'items' key")

			// Check the item structure
			if tt.expectedItemCount > 0 {
				itemsMap, ok := items.(map[string]interface{})
				require.True(t, ok, "Items should be a map[string]interface{}")

				itemList, ok := itemsMap["item"]
				require.True(t, ok, "Items should contain 'item' key")

				// For single item, it will be a map, for multiple items it would be a slice
				if tt.expectedItemCount == 1 {
					item, ok := itemList.(map[string]interface{})
					require.True(t, ok, "Single item should be a map[string]interface{}")

					// Verify all expected attributes
					for key, expectedValue := range tt.expectedAttributes {
						value, exists := item["-"+key] // XML attributes are prefixed with "-"
						assert.True(t, exists, "Item should have attribute '%s'", key)
						assert.Equal(t, expectedValue, value, "Attribute '%s' should have value '%s'", key, expectedValue)
					}

					// Check for output element
					output, ok := item["output"]
					assert.True(t, ok, "Item should have 'output' element")
					outputMap, ok := output.(map[string]interface{})
					assert.True(t, ok, "Output should be a map[string]interface{}")
					assert.Equal(t, "out", outputMap["-name"], "Output name should be 'out'")
				} else {
					// Handle multiple items case
					items, ok := itemList.([]interface{})
					assert.True(t, ok, "Multiple items should be a []interface{}")
					assert.Equal(t, tt.expectedItemCount, len(items), "Should have expected number of items")

					// Verify first item in multiple items
					if tt.name == "multiple installs data" {
						firstItem, ok := items[0].(map[string]interface{})
						assert.True(t, ok, "Item should be a map[string]interface{}")
						assert.Equal(t, "0", firstItem["-attrPath"], "First item should have attrPath '0'")
						assert.Equal(t, "git", firstItem["-pname"], "First item should have pname 'git'")

						// Verify last item in multiple items
						lastItem, ok := items[2].(map[string]interface{})
						assert.True(t, ok, "Item should be a map[string]interface{}")
						assert.Equal(t, "2", lastItem["-attrPath"], "Last item should have attrPath '2'")
						assert.Equal(t, "go", lastItem["-pname"], "Last item should have pname 'go'")
					}
				}
			}
		})
	}
}

// TestNewParser ensures that the New function returns a properly initialized parser
func TestNewParser(t *testing.T) {
	t.Parallel()
	p := New()
	assert.NotNil(t, p, "New should return a non-nil parser")
	assert.Nil(t, p.scanner, "New parser should have nil scanner")
	assert.Equal(t, "", p.lastReadLine, "New parser should have empty lastReadLine")
}

// TestParserSingleton ensures that the Parser singleton is properly initialized
func TestParserSingleton(t *testing.T) {
	t.Parallel()
	assert.NotNil(t, Parser, "Parser singleton should be non-nil")
}
