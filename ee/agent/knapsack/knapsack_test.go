package knapsack

import (
	"testing"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/stretchr/testify/require"
)

func TestMergeEnrollmentDetails(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		old      types.EnrollmentDetails
		new      types.EnrollmentDetails
		expected types.EnrollmentDetails
	}{
		{
			name: "empty old, populated new",
			old:  types.EnrollmentDetails{},
			new: types.EnrollmentDetails{
				OSPlatform:     "darwin",
				OSVersion:      "10.15.7",
				Hostname:       "test-host",
				OsqueryVersion: "5.0.1",
			},
			expected: types.EnrollmentDetails{
				OSPlatform:     "darwin",
				OSVersion:      "10.15.7",
				Hostname:       "test-host",
				OsqueryVersion: "5.0.1",
			},
		},
		{
			name: "populated old, empty new",
			old: types.EnrollmentDetails{
				OSPlatform:     "darwin",
				OSVersion:      "10.15.7",
				Hostname:       "test-host",
				OsqueryVersion: "5.0.1",
			},
			new: types.EnrollmentDetails{},
			expected: types.EnrollmentDetails{
				OSPlatform:     "darwin",
				OSVersion:      "10.15.7",
				Hostname:       "test-host",
				OsqueryVersion: "5.0.1",
			},
		},
		{
			name: "partial update",
			old: types.EnrollmentDetails{
				OSPlatform:     "darwin",
				OSVersion:      "10.15.7",
				Hostname:       "old-host",
				OsqueryVersion: "5.0.1",
				HardwareModel:  "MacBookPro16,1",
			},
			new: types.EnrollmentDetails{
				Hostname:       "new-host",
				OsqueryVersion: "5.0.2",
			},
			expected: types.EnrollmentDetails{
				OSPlatform:     "darwin",
				OSVersion:      "10.15.7",
				Hostname:       "new-host",
				OsqueryVersion: "5.0.2",
				HardwareModel:  "MacBookPro16,1",
			},
		},
		{
			name: "complete update",
			old: types.EnrollmentDetails{
				OSPlatform:                "darwin",
				OSPlatformLike:            "darwin",
				OSVersion:                 "10.15.7",
				OSBuildID:                 "19H2",
				Hostname:                  "old-host",
				HardwareSerial:            "C02XL0GYJGH5",
				HardwareModel:             "MacBookPro16,1",
				HardwareVendor:            "Apple Inc.",
				HardwareUUID:              "A1B2C3D4-E5F6-7890-ABCD-EF1234567890",
				OsqueryVersion:            "5.0.1",
				LauncherVersion:           "0.15.0",
				GOOS:                      "darwin",
				GOARCH:                    "amd64",
				LauncherLocalKey:          "key1",
				LauncherHardwareKey:       "key2",
				LauncherHardwareKeySource: "source1",
				OSName:                    "macOS",
			},
			new: types.EnrollmentDetails{
				OSPlatform:                "darwin",
				OSPlatformLike:            "darwin",
				OSVersion:                 "11.0.0",
				OSBuildID:                 "20A2411",
				Hostname:                  "new-host",
				HardwareSerial:            "C02XL0GYJGH6",
				HardwareModel:             "MacBookPro17,1",
				HardwareVendor:            "Apple Inc.",
				HardwareUUID:              "B2C3D4E5-F6G7-8901-BCDE-F01234567891",
				OsqueryVersion:            "5.0.2",
				LauncherVersion:           "0.16.0",
				GOOS:                      "darwin",
				GOARCH:                    "arm64",
				LauncherLocalKey:          "key3",
				LauncherHardwareKey:       "key4",
				LauncherHardwareKeySource: "source2",
				OSName:                    "macOS",
			},
			expected: types.EnrollmentDetails{
				OSPlatform:                "darwin",
				OSPlatformLike:            "darwin",
				OSVersion:                 "11.0.0",
				OSBuildID:                 "20A2411",
				Hostname:                  "new-host",
				HardwareSerial:            "C02XL0GYJGH6",
				HardwareModel:             "MacBookPro17,1",
				HardwareVendor:            "Apple Inc.",
				HardwareUUID:              "B2C3D4E5-F6G7-8901-BCDE-F01234567891",
				OsqueryVersion:            "5.0.2",
				LauncherVersion:           "0.16.0",
				GOOS:                      "darwin",
				GOARCH:                    "arm64",
				LauncherLocalKey:          "key3",
				LauncherHardwareKey:       "key4",
				LauncherHardwareKeySource: "source2",
				OSName:                    "macOS",
			},
		},
		{
			name: "empty strings don't overwrite",
			old: types.EnrollmentDetails{
				OSPlatform:     "darwin",
				OSVersion:      "10.15.7",
				Hostname:       "test-host",
				OsqueryVersion: "5.0.1",
			},
			new: types.EnrollmentDetails{
				OSPlatform:     "",
				OSVersion:      "",
				Hostname:       "new-host",
				OsqueryVersion: "",
			},
			expected: types.EnrollmentDetails{
				OSPlatform:     "darwin",
				OSVersion:      "10.15.7",
				Hostname:       "new-host",
				OsqueryVersion: "5.0.1",
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := mergeEnrollmentDetails(tc.old, tc.new)
			require.Equal(t, tc.expected, result)
		})
	}
}
