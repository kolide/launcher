//go:build darwin
// +build darwin

package plist

import (
	"bytes"
	_ "embed"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed test-data/diskutil-list.txt
var diskutilListData string

//go:embed test-data/powermetrics.txt
var powermetricsData string

//go:embed test-data/diskutil-apfs.txt
var diskutilApfsData string

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
			name:              "diskutil list data",
			input:             []byte(diskutilListData),
			expectedItemCount: 4, // Number of whole disks in the sample data
			expectedAttributes: map[string]interface{}{
				"AllDisks":   []interface{}{"disk0", "disk0s1", "disk0s2", "disk0s3", "disk1", "disk1s1", "disk1s2", "disk1s3", "disk1s4", "disk2", "disk2s1", "disk2s2", "disk3", "disk3s1", "disk3s1s1", "disk3s2", "disk3s3", "disk3s4", "disk3s5", "disk3s6", "disk3s7"},
				"WholeDisks": []interface{}{"disk0", "disk1", "disk2", "disk3"},
				"Size":       uint64(2001111162880), // Size of first disk in the sample data (as uint64)
				"OSInternal": false,                 // OSInternal value of first disk
				"Content":    "GUID_partition_scheme",
			},
			expectedErr: false,
		},
		{
			name:              "powermetrics data",
			input:             []byte(powermetricsData),
			expectedItemCount: 237, // Number of tasks in the sample data
			expectedAttributes: map[string]interface{}{
				"is_delta":       true,                                                  // Boolean value
				"elapsed_ns":     uint64(5023936166),                                    // Integer value as uint64
				"hw_model":       "Mac15,9",                                             // String value
				"kern_osversion": "24E263",                                              // String value
				"kern_boottime":  uint64(1745000486),                                    // Integer value as uint64
				"timestamp":      time.Date(2025, time.May, 7, 13, 43, 37, 0, time.UTC), // Time value
			},
			expectedErr: false,
		},
		{
			name:              "diskutil apfs data",
			input:             []byte(diskutilApfsData),
			expectedItemCount: 3, // Number of containers in the sample data
			expectedAttributes: map[string]interface{}{
				"ContainerReference": "disk1",                                // String value
				"APFSContainerUUID":  "E9CF7965-DC84-4CCD-95B1-8E7899A2F41D", // String value
				"CapacityCeiling":    uint64(524288000),                      // Integer value as uint64
				"CapacityFree":       uint64(504295424),                      // Integer value as uint64
				"Fusion":             false,                                  // Boolean value
				"Size":               uint64(524288000),                      // Integer value as uint64
				"DeviceIdentifier":   "disk0s1",                              // String value
				"DiskUUID":           "B02C7A05-B7E8-469E-A570-49F223D4935F", // String value
			},
			expectedErr: false,
		},
		{
			name:              "malformed plist",
			input:             []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?><plist><dict>malformed"),
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

			// Different handling based on the test case
			if tt.name == "diskutil list data" {
				// Check if we have AllDisksAndPartitions
				allDisksAndPartitions, ok := resultMap["AllDisksAndPartitions"]
				require.True(t, ok, "Result should contain 'AllDisksAndPartitions' key")

				// Check the AllDisksAndPartitions structure
				allDisksSlice, ok := allDisksAndPartitions.([]interface{})
				require.True(t, ok, "AllDisksAndPartitions should be a []interface{}")
				assert.Equal(t, tt.expectedItemCount, len(allDisksSlice), "Should have expected number of disks")

				// Check for WholeDisks
				wholeDisks, ok := resultMap["WholeDisks"]
				require.True(t, ok, "Result should contain 'WholeDisks' key")
				wholeDisksSlice, ok := wholeDisks.([]interface{})
				require.True(t, ok, "WholeDisks should be a []interface{}")
				assert.Equal(t, 4, len(wholeDisksSlice), "Should have 4 whole disks")

				// Check for AllDisks
				allDisks, ok := resultMap["AllDisks"]
				require.True(t, ok, "Result should contain 'AllDisks' key")
				// Fix: Use different variable names to avoid redeclaration
				allDisksArray, okAllDisks := allDisks.([]interface{})
				require.True(t, okAllDisks, "AllDisks should be a []interface{}")
				assert.Equal(t, 21, len(allDisksArray), "Should have 21 disks in AllDisks")

				// Verify specific expected attributes
				for key, expectedValue := range tt.expectedAttributes {
					switch key {
					case "AllDisks", "WholeDisks":
						// Already checked above
						continue
					case "Size", "OSInternal", "Content":
						// These are in the first disk in AllDisksAndPartitions
						firstDisk, ok := allDisksSlice[0].(map[string]interface{})
						require.True(t, ok, "First disk should be a map[string]interface{}")
						value, exists := firstDisk[key]
						assert.True(t, exists, "First disk should have '%s'", key)

						// Special handling for Size which might be uint64 vs int64
						if key == "Size" {
							// Convert both to uint64 for comparison
							var actualSize uint64
							switch v := value.(type) {
							case uint64:
								actualSize = v
							case int64:
								actualSize = uint64(v)
							case int:
								actualSize = uint64(v)
							}

							var expectedSize uint64
							switch v := expectedValue.(type) {
							case uint64:
								expectedSize = v
							case int64:
								expectedSize = uint64(v)
							case int:
								expectedSize = uint64(v)
							}

							assert.Equal(t, expectedSize, actualSize, "First disk 'Size' should have correct value")
						} else {
							assert.Equal(t, expectedValue, value, "First disk '%s' should have value '%v'", key, expectedValue)
						}
					default:
						value, exists := resultMap[key]
						assert.True(t, exists, "Result should have '%s'", key)
						assert.Equal(t, expectedValue, value, "'%s' should have value '%v'", key, expectedValue)
					}
				}

				// Check for specific disk details
				firstDisk, ok := allDisksSlice[0].(map[string]interface{})
				require.True(t, ok, "First disk should be a map[string]interface{}")
				assert.Equal(t, "disk0", firstDisk["DeviceIdentifier"], "First disk should have DeviceIdentifier 'disk0'")

				// Check for partitions in the first disk
				partitions, ok := firstDisk["Partitions"]
				require.True(t, ok, "First disk should have 'Partitions' key")
				partitionsSlice, ok := partitions.([]interface{})
				require.True(t, ok, "Partitions should be a []interface{}")
				assert.Equal(t, 3, len(partitionsSlice), "First disk should have 3 partitions")

				// Check first partition details
				firstPartition, ok := partitionsSlice[0].(map[string]interface{})
				require.True(t, ok, "First partition should be a map[string]interface{}")
				assert.Equal(t, "disk0s1", firstPartition["DeviceIdentifier"], "First partition should have DeviceIdentifier 'disk0s1'")
				assert.Equal(t, "Apple_APFS_ISC", firstPartition["Content"], "First partition should have Content 'Apple_APFS_ISC'")
			} else if tt.name == "powermetrics data" {
				// Check for tasks array
				tasks, ok := resultMap["tasks"]
				require.True(t, ok, "Result should contain 'tasks' key")
				tasksSlice, ok := tasks.([]interface{})
				require.True(t, ok, "Tasks should be a []interface{}")
				assert.Equal(t, tt.expectedItemCount, len(tasksSlice), "Should have expected number of tasks")

				// Check first task details
				firstTask, ok := tasksSlice[0].(map[string]interface{})
				require.True(t, ok, "First task should be a map[string]interface{}")

				// Use uint64 for pid
				pid, ok := firstTask["pid"].(uint64)
				require.True(t, ok, "pid should be a uint64")
				assert.Equal(t, uint64(576), pid, "First task should have pid 576")

				assert.Equal(t, "WindowServer", firstTask["name"], "First task should have name 'WindowServer'")

				// Check for timer_wakeups in first task (nested array)
				timerWakeups, ok := firstTask["timer_wakeups"]
				require.True(t, ok, "First task should have 'timer_wakeups' key")
				timerWakeupsSlice, ok := timerWakeups.([]interface{})
				require.True(t, ok, "timer_wakeups should be a []interface{}")
				assert.Equal(t, 2, len(timerWakeupsSlice), "First task should have 2 timer_wakeups entries")

				// Check first timer_wakeup details
				firstTimerWakeup, ok := timerWakeupsSlice[0].(map[string]interface{})
				require.True(t, ok, "First timer_wakeup should be a map[string]interface{}")

				// Use uint64 for interval_ns
				intervalNs, ok := firstTimerWakeup["interval_ns"].(uint64)
				require.True(t, ok, "interval_ns should be a uint64")
				assert.Equal(t, uint64(2000000), intervalNs, "First timer_wakeup should have interval_ns 2000000")

				// Verify specific expected attributes (top-level)
				for key, expectedValue := range tt.expectedAttributes {
					value, exists := resultMap[key]
					assert.True(t, exists, "Result should have '%s'", key)

					// Special handling for different types
					switch key {
					case "is_delta":
						boolValue, ok := value.(bool)
						require.True(t, ok, "is_delta should be a bool")
						assert.Equal(t, expectedValue, boolValue, "'%s' should have value '%v'", key, expectedValue)
					case "elapsed_ns", "kern_boottime":
						// Handle uint64 values
						uintValue, ok := value.(uint64)
						require.True(t, ok, "%s should be a uint64", key)
						assert.Equal(t, expectedValue, uintValue, "'%s' should have value '%v'", key, expectedValue)
					case "timestamp":
						// Handle time.Time values
						timeValue, ok := value.(time.Time)
						require.True(t, ok, "timestamp should be a time.Time")
						expectedTime, ok := expectedValue.(time.Time)
						require.True(t, ok, "expected timestamp should be a time.Time")
						assert.Equal(t, expectedTime, timeValue, "'%s' should have value '%v'", key, expectedValue)
					default:
						assert.Equal(t, expectedValue, value, "'%s' should have value '%v'", key, expectedValue)
					}
				}

				// Check for real values in the first task
				cputimePerS, ok := firstTask["cputime_ms_per_s"]
				require.True(t, ok, "First task should have 'cputime_ms_per_s' key")
				_, ok = cputimePerS.(float64)
				require.True(t, ok, "cputime_ms_per_s should be a float64")
			} else if tt.name == "diskutil apfs data" {
				// Check for Containers array
				containers, ok := resultMap["Containers"]
				require.True(t, ok, "Result should contain 'Containers' key")
				containersSlice, ok := containers.([]interface{})
				require.True(t, ok, "Containers should be a []interface{}")
				assert.Equal(t, tt.expectedItemCount, len(containersSlice), "Should have expected number of containers")

				// Check first container details
				firstContainer, ok := containersSlice[0].(map[string]interface{})
				require.True(t, ok, "First container should be a map[string]interface{}")

				// Check container attributes
				assert.Equal(t, "E9CF7965-DC84-4CCD-95B1-8E7899A2F41D", firstContainer["APFSContainerUUID"], "First container should have correct APFSContainerUUID")
				assert.Equal(t, "disk1", firstContainer["ContainerReference"], "First container should have ContainerReference 'disk1'")
				assert.Equal(t, false, firstContainer["Fusion"], "First container should have Fusion 'false'")

				// Check capacity values (uint64)
				capacityCeiling, ok := firstContainer["CapacityCeiling"].(uint64)
				require.True(t, ok, "CapacityCeiling should be a uint64")
				assert.Equal(t, uint64(524288000), capacityCeiling, "First container should have CapacityCeiling 524288000")

				capacityFree, ok := firstContainer["CapacityFree"].(uint64)
				require.True(t, ok, "CapacityFree should be a uint64")
				assert.Equal(t, uint64(504295424), capacityFree, "First container should have CapacityFree 504295424")

				// Check for PhysicalStores array
				physicalStores, ok := firstContainer["PhysicalStores"]
				require.True(t, ok, "First container should have 'PhysicalStores' key")
				physicalStoresSlice, ok := physicalStores.([]interface{})
				require.True(t, ok, "PhysicalStores should be a []interface{}")
				assert.Equal(t, 1, len(physicalStoresSlice), "First container should have 1 physical store")

				// Check first physical store details
				firstStore, ok := physicalStoresSlice[0].(map[string]interface{})
				require.True(t, ok, "First physical store should be a map[string]interface{}")
				assert.Equal(t, "disk0s1", firstStore["DeviceIdentifier"], "First physical store should have DeviceIdentifier 'disk0s1'")
				assert.Equal(t, "B02C7A05-B7E8-469E-A570-49F223D4935F", firstStore["DiskUUID"], "First physical store should have correct DiskUUID")

				// Check size (uint64)
				storeSize, ok := firstStore["Size"].(uint64)
				require.True(t, ok, "Size should be a uint64")
				assert.Equal(t, uint64(524288000), storeSize, "First physical store should have Size 524288000")

				// Check for Volumes array
				volumes, ok := firstContainer["Volumes"]
				require.True(t, ok, "First container should have 'Volumes' key")
				volumesSlice, ok := volumes.([]interface{})
				require.True(t, ok, "Volumes should be a []interface{}")
				assert.Equal(t, 4, len(volumesSlice), "First container should have 4 volumes")

				// Check first volume details
				firstVolume, ok := volumesSlice[0].(map[string]interface{})
				require.True(t, ok, "First volume should be a map[string]interface{}")
				assert.Equal(t, "6F9AA06F-1FA2-4F56-A0BD-61AE5572A016", firstVolume["APFSVolumeUUID"], "First volume should have correct APFSVolumeUUID")
				assert.Equal(t, "disk1s1", firstVolume["DeviceIdentifier"], "First volume should have DeviceIdentifier 'disk1s1'")
				assert.Equal(t, "iSCPreboot", firstVolume["Name"], "First volume should have Name 'iSCPreboot'")

				// Check boolean values
				assert.Equal(t, false, firstVolume["Encryption"], "First volume should have Encryption 'false'")
				assert.Equal(t, false, firstVolume["FileVault"], "First volume should have FileVault 'false'")
				assert.Equal(t, false, firstVolume["Locked"], "First volume should have Locked 'false'")

				// Check capacity values (uint64)
				volumeCapacityInUse, ok := firstVolume["CapacityInUse"].(uint64)
				require.True(t, ok, "CapacityInUse should be a uint64")
				assert.Equal(t, uint64(5668864), volumeCapacityInUse, "First volume should have CapacityInUse 5668864")

				// Check for Roles array
				roles, ok := firstVolume["Roles"]
				require.True(t, ok, "First volume should have 'Roles' key")
				rolesSlice, ok := roles.([]interface{})
				require.True(t, ok, "Roles should be a []interface{}")
				assert.Equal(t, 1, len(rolesSlice), "First volume should have 1 role")
				assert.Equal(t, "Preboot", rolesSlice[0], "First volume should have role 'Preboot'")

				// Verify specific expected attributes
				for key, expectedValue := range tt.expectedAttributes {
					var value interface{}
					var exists bool

					// Check in different places based on the key
					switch key {
					case "ContainerReference", "APFSContainerUUID", "CapacityCeiling", "CapacityFree", "Fusion":
						// These are in the first container
						value, exists = firstContainer[key]
						assert.True(t, exists, "First container should have '%s'", key)
					case "DeviceIdentifier", "DiskUUID", "Size":
						// These are in the first physical store
						value, exists = firstStore[key]
						assert.True(t, exists, "First physical store should have '%s'", key)
					default:
						// Try to find in the top level
						value, exists = resultMap[key]
						assert.True(t, exists, "Result should have '%s'", key)
					}

					// Special handling for different types
					switch key {
					case "Fusion":
						boolValue, ok := value.(bool)
						require.True(t, ok, "%s should be a bool", key)
						assert.Equal(t, expectedValue, boolValue, "'%s' should have value '%v'", key, expectedValue)
					case "CapacityCeiling", "CapacityFree", "Size":
						// Handle uint64 values
						uintValue, ok := value.(uint64)
						require.True(t, ok, "%s should be a uint64", key)
						expectedUint, ok := expectedValue.(uint64)
						require.True(t, ok, "expected %s should be a uint64", key)
						assert.Equal(t, expectedUint, uintValue, "'%s' should have value '%v'", key, expectedValue)
					default:
						assert.Equal(t, expectedValue, value, "'%s' should have value '%v'", key, expectedValue)
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
}

// TestParserSingleton ensures that the Parser singleton is properly initialized
func TestParserSingleton(t *testing.T) {
	t.Parallel()
	assert.NotNil(t, Parser, "Parser singleton should be non-nil")
}
