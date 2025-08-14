package knapsack

import (
	"encoding/json"
	"testing"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/pkg/log/multislogger"
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

func TestSaveRegistration(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName           string
		expectedRegistrationId string
		expectedMunemo         string
		expectedNodeKey        string
		expectedEnrollSecret   string
	}{
		{
			testCaseName:           "all data set, default registration id",
			expectedRegistrationId: types.DefaultRegistrationID,
			expectedMunemo:         "test_munemo",
			expectedNodeKey:        "test_node_key",
			expectedEnrollSecret:   "test_jwt",
		},
		{
			testCaseName:           "all data set, non-default registration id",
			expectedRegistrationId: ulid.New(),
			expectedMunemo:         "test_munemo",
			expectedNodeKey:        "test_node_key",
			expectedEnrollSecret:   "test_jwt",
		},
		{
			testCaseName:           "no enroll secret, default registration ID",
			expectedRegistrationId: types.DefaultRegistrationID,
			expectedMunemo:         "test_munemo",
			expectedNodeKey:        "test_node_key",
			expectedEnrollSecret:   "",
		},
		{
			testCaseName:           "no enroll secret, non-default registration ID",
			expectedRegistrationId: ulid.New(),
			expectedMunemo:         "test_munemo",
			expectedNodeKey:        "test_node_key",
			expectedEnrollSecret:   "",
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			// Set up our stores
			configStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String())
			require.NoError(t, err)
			registrationStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.RegistrationStore.String())
			require.NoError(t, err)

			// Set up our knapsack
			testKnapsack := New(map[storage.Store]types.KVStore{
				storage.ConfigStore:       configStore,
				storage.RegistrationStore: registrationStore,
			}, nil, nil, multislogger.New(), multislogger.New())

			require.NoError(t, testKnapsack.SaveRegistration(tt.expectedRegistrationId, tt.expectedMunemo, tt.expectedNodeKey, tt.expectedEnrollSecret))

			// Confirm that the node key was stored
			expectedNodeKeyKey := storage.KeyByIdentifier(nodeKeyKey, storage.IdentifierTypeRegistration, []byte(tt.expectedRegistrationId))
			storedKey, err := configStore.Get(expectedNodeKeyKey)
			require.NoError(t, err)
			require.Equal(t, tt.expectedNodeKey, string(storedKey))

			// Confirm that the registration was stored
			rawStoredRegistration, err := registrationStore.Get([]byte(tt.expectedRegistrationId))
			require.NoError(t, err)
			var storedRegistration types.Registration
			require.NoError(t, json.Unmarshal(rawStoredRegistration, &storedRegistration))
			require.Equal(t, tt.expectedRegistrationId, storedRegistration.RegistrationID)
			require.Equal(t, tt.expectedMunemo, storedRegistration.Munemo)
			require.Equal(t, tt.expectedNodeKey, storedRegistration.NodeKey)
			require.Equal(t, tt.expectedEnrollSecret, storedRegistration.EnrollmentSecret)
		})
	}
}
