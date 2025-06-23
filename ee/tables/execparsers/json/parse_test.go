package json

import (
	"bytes"
	_ "embed"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed test-data/kolide_lsblk.json
var lsblkData string

//go:embed test-data/kolide_nftables.json
var nftablesData string

func TestParse(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name               string
		input              []byte
		expectedItemCount  int
		expectedAttributes map[string]interface{}
		expectedErr        bool
	}{
		{
			name:              "empty input",
			input:             []byte(""),
			expectedItemCount: 0,
			expectedErr:       true,
		},
		{
			name:              "lsblk data",
			input:             []byte(lsblkData),
			expectedItemCount: 4,
			expectedAttributes: map[string]interface{}{
				"name": "/dev/sda",
			},
			expectedErr: false,
		},
		{
			name:              "nftables data",
			input:             []byte(nftablesData),
			expectedItemCount: 6,
			expectedAttributes: map[string]interface{}{
				"family": "ip",
			},
			expectedErr: false,
		},
		{
			name:              "malformed JSON",
			input:             []byte("{\"key\": \"value\""),
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

			if tt.name == "lsblk data" {
				// Check blockdevices
				blockdevices, ok := resultMap["blockdevices"]
				require.True(t, ok, "Result should contain 'blockdevices' key")

				// Check the blockdevices structure
				blockdevicesList, ok := blockdevices.([]interface{})
				require.True(t, ok, "blockdevices should be a []interface{}")
				assert.Equal(t, tt.expectedItemCount, len(blockdevicesList), "Should have expected number of blockdevices")

				// Check first blockdevice (/dev/sda)
				firstDevice, ok := blockdevicesList[0].(map[string]interface{})
				require.True(t, ok, "First blockdevice should be a map[string]interface{}")
				assert.Equal(t, tt.expectedAttributes["name"], firstDevice["name"], "First blockdevice should have name '/dev/sda'")
				assert.Nil(t, firstDevice["fstype"], "First blockdevice should have fstype nil")

				// Check children of first blockdevice
				children, ok := firstDevice["children"]
				require.True(t, ok, "First blockdevice should have 'children' key")
				childrenList, ok := children.([]interface{})
				require.True(t, ok, "children should be a []interface{}")
				assert.Equal(t, 3, len(childrenList), "First blockdevice should have 3 children")

				// Check first child of first blockdevice (/dev/sda1)
				firstChild, ok := childrenList[0].(map[string]interface{})
				require.True(t, ok, "First child should be a map[string]interface{}")
				assert.Equal(t, "/dev/sda1", firstChild["name"], "First child should have name '/dev/sda1'")
				assert.Nil(t, firstChild["fstype"], "First child should have fstype nil")

				// Check second child of first blockdevice (/dev/sda2)
				secondChild, ok := childrenList[1].(map[string]interface{})
				require.True(t, ok, "Second child should be a map[string]interface{}")
				assert.Equal(t, "/dev/sda2", secondChild["name"], "Second child should have name '/dev/sda2'")
				assert.Equal(t, "vfat", secondChild["fstype"], "Second child should have fstype 'vfat'")
				assert.Equal(t, "FAT16", secondChild["fsver"], "Second child should have fsver 'FAT16'")
				assert.Equal(t, "EFI", secondChild["label"], "Second child should have label 'EFI'")

				// Check third child of first blockdevice (/dev/sda3)
				thirdChild, ok := childrenList[2].(map[string]interface{})
				require.True(t, ok, "Third child should be a map[string]interface{}")
				assert.Equal(t, "/dev/sda3", thirdChild["name"], "Third child should have name '/dev/sda3'")
				assert.Equal(t, "xfs", thirdChild["fstype"], "Third child should have fstype 'xfs'")
				assert.Equal(t, "ROOT", thirdChild["label"], "Third child should have label 'ROOT'")

				// Check second blockdevice (/dev/sdb)
				secondDevice, ok := blockdevicesList[1].(map[string]interface{})
				require.True(t, ok, "Second blockdevice should be a map[string]interface{}")
				assert.Equal(t, "/dev/sdb", secondDevice["name"], "Second blockdevice should have name '/dev/sdb'")

				// Check third blockdevice (/dev/sdc) and its nested children
				thirdDevice, ok := blockdevicesList[2].(map[string]interface{})
				require.True(t, ok, "Third blockdevice should be a map[string]interface{}")
				assert.Equal(t, "/dev/sdc", thirdDevice["name"], "Third blockdevice should have name '/dev/sdc'")

				// Check children of third blockdevice
				thirdDeviceChildren, ok := thirdDevice["children"].([]interface{})
				require.True(t, ok, "Third blockdevice should have children as []interface{}")

				// Check second child of third blockdevice (/dev/sdc2)
				thirdDeviceSecondChild, ok := thirdDeviceChildren[1].(map[string]interface{})
				require.True(t, ok, "Second child of third blockdevice should be a map[string]interface{}")
				assert.Equal(t, "/dev/sdc2", thirdDeviceSecondChild["name"], "Second child of third blockdevice should have name '/dev/sdc2'")
				assert.Equal(t, "LVM2_member", thirdDeviceSecondChild["fstype"], "Second child of third blockdevice should have fstype 'LVM2_member'")

				// Check grandchildren (nested children)
				grandchildren, ok := thirdDeviceSecondChild["children"]
				require.True(t, ok, "Second child of third blockdevice should have 'children' key")
				grandchildrenList, ok := grandchildren.([]interface{})
				require.True(t, ok, "Grandchildren should be a []interface{}")
				assert.Equal(t, 4, len(grandchildrenList), "Should have 4 grandchildren")

				// Check first grandchild
				firstGrandchild, ok := grandchildrenList[0].(map[string]interface{})
				require.True(t, ok, "First grandchild should be a map[string]interface{}")
				assert.Equal(t, "/dev/mapper/LvmEncrypt-lvmcryptroot", firstGrandchild["name"], "First grandchild should have correct name")
				assert.Equal(t, "crypto_LUKS", firstGrandchild["fstype"], "First grandchild should have fstype 'crypto_LUKS'")

			} else if tt.name == "nftables data" {
				// Check nftables
				nftables, ok := resultMap["nftables"]
				require.True(t, ok, "Result should contain 'nftables' key")

				// Check the nftables structure
				nftablesList, ok := nftables.([]interface{})
				require.True(t, ok, "nftables should be a []interface{}")
				assert.Equal(t, tt.expectedItemCount, len(nftablesList), "Should have expected number of nftables items")

				// Check metainfo item (first item)
				metainfoItem, ok := nftablesList[0].(map[string]interface{})
				require.True(t, ok, "First item should be a map[string]interface{}")
				metainfo, ok := metainfoItem["metainfo"]
				require.True(t, ok, "First item should have 'metainfo' key")
				metainfoMap, ok := metainfo.(map[string]interface{})
				require.True(t, ok, "metainfo should be a map[string]interface{}")
				assert.Equal(t, "1.0.2", metainfoMap["version"], "Metainfo should have version '1.0.2'")
				assert.Equal(t, "Lester Gooch", metainfoMap["release_name"], "Metainfo should have correct release_name")
				assert.Equal(t, float64(1), metainfoMap["json_schema_version"], "Metainfo should have json_schema_version 1")

				// Check table item (second item)
				tableItem, ok := nftablesList[1].(map[string]interface{})
				require.True(t, ok, "Second item should be a map[string]interface{}")
				table, ok := tableItem["table"]
				require.True(t, ok, "Second item should have 'table' key")
				tableMap, ok := table.(map[string]interface{})
				require.True(t, ok, "table should be a map[string]interface{}")
				assert.Equal(t, tt.expectedAttributes["family"], tableMap["family"], "Table should have family 'ip'")
				assert.Equal(t, "filter", tableMap["name"], "Table should have name 'filter'")
				assert.Equal(t, float64(1), tableMap["handle"], "Table should have handle 1")

				// Check chain items (third and fifth items)
				chainItem1, ok := nftablesList[2].(map[string]interface{})
				require.True(t, ok, "Third item should be a map[string]interface{}")
				chain1, ok := chainItem1["chain"]
				require.True(t, ok, "Third item should have 'chain' key")
				chain1Map, ok := chain1.(map[string]interface{})
				require.True(t, ok, "chain should be a map[string]interface{}")
				assert.Equal(t, "ip", chain1Map["family"], "Chain should have family 'ip'")
				assert.Equal(t, "filter", chain1Map["table"], "Chain should have table 'filter'")
				assert.Equal(t, "OUTPUT", chain1Map["name"], "Chain should have name 'OUTPUT'")

				// Check rule items (fourth and sixth items)
				ruleItem1, ok := nftablesList[3].(map[string]interface{})
				require.True(t, ok, "Fourth item should be a map[string]interface{}")
				rule1, ok := ruleItem1["rule"]
				require.True(t, ok, "Fourth item should have 'rule' key")
				rule1Map, ok := rule1.(map[string]interface{})
				require.True(t, ok, "rule should be a map[string]interface{}")
				assert.Equal(t, "ip", rule1Map["family"], "Rule should have family 'ip'")
				assert.Equal(t, "filter", rule1Map["table"], "Rule should have table 'filter'")
				assert.Equal(t, "OUTPUT", rule1Map["chain"], "Rule should have chain 'OUTPUT'")

				// Check rule expressions
				expr, ok := rule1Map["expr"]
				require.True(t, ok, "Rule should have 'expr' key")
				exprList, ok := expr.([]interface{})
				require.True(t, ok, "expr should be a []interface{}")
				assert.Equal(t, 3, len(exprList), "Rule should have 3 expressions")

				// Check first expression (match)
				firstExpr, ok := exprList[0].(map[string]interface{})
				require.True(t, ok, "First expression should be a map[string]interface{}")
				match, ok := firstExpr["match"]
				require.True(t, ok, "First expression should have 'match' key")
				matchMap, ok := match.(map[string]interface{})
				require.True(t, ok, "match should be a map[string]interface{}")
				assert.Equal(t, "==", matchMap["op"], "Match should have op '=='")
			}
		})
	}
}

// TestNewParser ensures that the New function returns a properly initialized parser
func TestNewParser(t *testing.T) {
	t.Parallel()
	p := New()
	assert.NotNil(t, p, "New should return a non-nil parser")
}

// TestParserSingleton ensures that the Parser singleton is properly initialized
func TestParserSingleton(t *testing.T) {
	t.Parallel()
	assert.NotNil(t, Parser, "Parser singleton should be non-nil")
}
