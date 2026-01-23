package knapsack

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	"github.com/kolide/launcher/ee/agent/types"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
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
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := mergeEnrollmentDetails(tc.old, tc.new)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestSaveEnrollment(t *testing.T) {
	t.Parallel()

	type testCase struct {
		testCaseName         string
		enrollmentId         string
		munemo               string
		expectedMunemo       string
		expectedNodeKey      string
		expectedEnrollSecret string
		errorExpected        bool
	}

	testCases := make([]testCase, 0)

	for _, isDefaultEnrollmentId := range []bool{true, false} {
		enrollmentId := types.DefaultEnrollmentID
		testCaseNameSuffix := " (default enrollment ID)"

		if !isDefaultEnrollmentId {
			enrollmentId = ulid.New()
			testCaseNameSuffix = " (non-default registration ID)"
		}

		testMunemo := ulid.New()
		enrollSecret := createTestEnrollSecret(t, testMunemo)

		testCases = append(testCases, []testCase{
			{
				testCaseName:         "all data set" + testCaseNameSuffix,
				enrollmentId:         enrollmentId,
				munemo:               testMunemo,
				expectedMunemo:       testMunemo,
				expectedNodeKey:      ulid.New(),
				expectedEnrollSecret: enrollSecret,
				errorExpected:        false,
			},
			{
				testCaseName:         "no enroll secret" + testCaseNameSuffix,
				enrollmentId:         enrollmentId,
				munemo:               testMunemo,
				expectedMunemo:       testMunemo,
				expectedNodeKey:      ulid.New(),
				expectedEnrollSecret: "",
				errorExpected:        false,
			},
			{
				testCaseName:         "no munemo given, but set in enrollment secret" + testCaseNameSuffix,
				enrollmentId:         enrollmentId,
				munemo:               "",
				expectedMunemo:       testMunemo,
				expectedNodeKey:      ulid.New(),
				expectedEnrollSecret: enrollSecret,
				errorExpected:        false,
			},
			{
				testCaseName:         "no munemo or enrollment secret given" + testCaseNameSuffix,
				enrollmentId:         enrollmentId,
				munemo:               "",
				expectedMunemo:       testMunemo,
				expectedNodeKey:      ulid.New(),
				expectedEnrollSecret: "",
				errorExpected:        true,
			},
			{
				testCaseName:         "no node key given" + testCaseNameSuffix,
				enrollmentId:         enrollmentId,
				munemo:               testMunemo,
				expectedMunemo:       testMunemo,
				expectedNodeKey:      "",
				expectedEnrollSecret: enrollSecret,
				errorExpected:        true,
			},
		}...)
	}

	for _, tt := range testCases {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			// Set up our stores
			configStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String())
			require.NoError(t, err)
			enrollmentStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.EnrollmentStore.String())
			require.NoError(t, err)

			// Set up our knapsack
			testKnapsack := New(map[storage.Store]types.KVStore{
				storage.ConfigStore:     configStore,
				storage.EnrollmentStore: enrollmentStore,
			}, nil, nil, multislogger.New(), multislogger.New())

			err = testKnapsack.SaveEnrollment(tt.enrollmentId, tt.munemo, tt.expectedNodeKey, tt.expectedEnrollSecret)
			if tt.errorExpected {
				require.Error(t, err)
				return // nothing else to test
			}
			require.NoError(t, err)

			// Confirm that the node key was stored
			expectedNodeKeyKey := storage.KeyByIdentifier(nodeKeyKey, storage.IdentifierTypeEnrollment, []byte(tt.enrollmentId))
			storedKey, err := configStore.Get(expectedNodeKeyKey)
			require.NoError(t, err)
			require.Equal(t, tt.expectedNodeKey, string(storedKey))

			// Confirm that the registration was stored
			rawStoredEnrollment, err := enrollmentStore.Get([]byte(tt.enrollmentId))
			require.NoError(t, err)
			var storedEnrollment types.Enrollment
			require.NoError(t, json.Unmarshal(rawStoredEnrollment, &storedEnrollment))
			require.Equal(t, tt.enrollmentId, storedEnrollment.EnrollmentID)
			require.Equal(t, tt.expectedMunemo, storedEnrollment.Munemo)
			require.Equal(t, tt.expectedNodeKey, storedEnrollment.NodeKey)
			require.Equal(t, tt.expectedEnrollSecret, storedEnrollment.EnrollmentSecret)
		})
	}
}

func TestEnsureEnrollmentStored(t *testing.T) {
	t.Parallel()

	type testCase struct {
		testCaseName           string
		enrollmentId           string
		nodeKeyStored          bool
		enrollmentSecretExists bool
		enrollmentSecretValid  bool
		enrollmentExists       bool
		successExpected        bool
	}

	testCases := make([]testCase, 0)

	for _, isDefaultEnrollmentId := range []bool{true, false} {
		enrollmentId := types.DefaultEnrollmentID
		testCaseNameSuffix := " (default enrollment ID)"

		if !isDefaultEnrollmentId {
			enrollmentId = ulid.New()
			testCaseNameSuffix = " (non-default enrollment ID)"
		}

		testCases = append(testCases, []testCase{
			{
				testCaseName:           "happy path, creating enrollment from scratch" + testCaseNameSuffix,
				enrollmentId:           enrollmentId,
				nodeKeyStored:          true,
				enrollmentSecretExists: true,
				enrollmentSecretValid:  true,
				enrollmentExists:       false,
				successExpected:        true,
			},
			{
				testCaseName:           "happy path, updating enrollment to add node key" + testCaseNameSuffix,
				enrollmentId:           enrollmentId,
				nodeKeyStored:          true,
				enrollmentSecretExists: false, // value does not matter for this test case, we should not need enrollment secret
				enrollmentSecretValid:  false, // value does not matter for this test case, we should not need enrollment secret
				enrollmentExists:       true,
				successExpected:        true,
			},
			{
				testCaseName:           "no node key" + testCaseNameSuffix,
				enrollmentId:           enrollmentId,
				nodeKeyStored:          false,
				enrollmentSecretExists: false, // value does not matter for this test case, we should not need enrollment secret
				enrollmentSecretValid:  false, // value does not matter for this test case, we should not need enrollment secret
				enrollmentExists:       false, // value does not matter for this test case
				successExpected:        false,
			},
			{
				testCaseName:           "no enrollment, and no enrollment secret" + testCaseNameSuffix,
				enrollmentId:           enrollmentId,
				nodeKeyStored:          true,
				enrollmentSecretExists: false,
				enrollmentSecretValid:  false, // value does not matter for this test case
				enrollmentExists:       false,
				successExpected:        false,
			},
			{
				testCaseName:           "no enrollment, and no valid enrollment secret" + testCaseNameSuffix,
				enrollmentId:           enrollmentId,
				nodeKeyStored:          true,
				enrollmentSecretExists: true,
				enrollmentSecretValid:  false,
				enrollmentExists:       false,
				successExpected:        false,
			},
		}...)
	}

	for _, tt := range testCases {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			// Set up our stores
			configStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String())
			require.NoError(t, err)
			enrollmentStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.EnrollmentStore.String())
			require.NoError(t, err)

			// Set up our knapsack
			mockFlags := typesmocks.NewFlags(t)
			testKnapsack := New(map[storage.Store]types.KVStore{
				storage.ConfigStore:     configStore,
				storage.EnrollmentStore: enrollmentStore,
			}, mockFlags, nil, multislogger.New(), multislogger.New())

			// Set up our test enrollment secret
			testMunemo := ulid.New()
			enrollSecret := ""
			if tt.enrollmentSecretExists {
				enrollSecret = "invalid-enroll-secret"
				if tt.enrollmentSecretValid {
					enrollSecret = createTestEnrollSecret(t, testMunemo)
				}
			}
			mockFlags.On("EnrollSecret").Return(enrollSecret, nil).Maybe()
			mockFlags.On("EnrollSecretPath").Return("", nil).Maybe() // We never expect to read the enrollment secret from here

			// Set up enrollment with node key missing
			if tt.enrollmentExists {
				// Save the enrollment
				r := types.Enrollment{
					EnrollmentID:     tt.enrollmentId,
					Munemo:           testMunemo,
					NodeKey:          "",
					EnrollmentSecret: enrollSecret,
				}
				rawEnrollment, err := json.Marshal(r)
				require.NoError(t, err)
				require.NoError(t, enrollmentStore.Set([]byte(tt.enrollmentId), rawEnrollment))

				// Confirm enrollment was saved as expected
				rawStoredEnrollment, err := enrollmentStore.Get([]byte(tt.enrollmentId))
				require.NoError(t, err)
				var storedEnrollment types.Enrollment
				require.NoError(t, json.Unmarshal(rawStoredEnrollment, &storedEnrollment))
				require.Equal(t, "", storedEnrollment.NodeKey)
			}

			// Finally, set up our new node key. If stored, it should be stored in the config store only.
			nodeKey := ulid.New()
			if tt.nodeKeyStored {
				nodeKeyKeyForEnrollment := storage.KeyByIdentifier(nodeKeyKey, storage.IdentifierTypeEnrollment, []byte(tt.enrollmentId))
				require.NoError(t, configStore.Set(nodeKeyKeyForEnrollment, []byte(nodeKey)))
				savedNodeKey, err := testKnapsack.NodeKey(tt.enrollmentId)
				require.NoError(t, err, "could not store node key during test setup")
				require.Equal(t, nodeKey, savedNodeKey)
			}

			// Now we're ready to test -- call the function, then check to make sure the enrollment
			// looks how we expect.
			err = testKnapsack.EnsureEnrollmentStored(tt.enrollmentId)
			if tt.successExpected {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}

			rawUpdatedEnrollment, err := enrollmentStore.Get([]byte(tt.enrollmentId))
			require.NoError(t, err)
			if tt.successExpected {
				var updatedEnrollment types.Enrollment
				require.NoError(t, json.Unmarshal(rawUpdatedEnrollment, &updatedEnrollment))

				// All data on the enrollment should be correct
				require.Equal(t, nodeKey, updatedEnrollment.NodeKey)
				require.Equal(t, enrollSecret, updatedEnrollment.EnrollmentSecret)
				require.Equal(t, tt.enrollmentId, updatedEnrollment.EnrollmentID)
				require.Equal(t, testMunemo, updatedEnrollment.Munemo)

				return
			}

			// Success was not expected.
			// If the registration already existed -- we expect that the node key was not updated.
			if tt.enrollmentExists {
				var updatedEnrollment types.Enrollment
				require.NoError(t, json.Unmarshal(rawUpdatedEnrollment, &updatedEnrollment))
				require.Equal(t, "", updatedEnrollment.NodeKey)
				return
			}

			// If the registration did not already exist, then we should not have created it at all.
			require.Nil(t, rawUpdatedEnrollment)
		})
	}
}

func TestNodeKey(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName    string
		enrollmentId    string
		expectedNodeKey string
	}{
		{
			testCaseName:    "default enrollment id",
			enrollmentId:    types.DefaultEnrollmentID,
			expectedNodeKey: "test_node_key",
		},
		{
			testCaseName:    "non-default enrollment id",
			enrollmentId:    ulid.New(),
			expectedNodeKey: "test_node_key_2",
		},
	} {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			// Set up our stores
			configStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String())
			require.NoError(t, err)
			enrollmentStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.EnrollmentStore.String())
			require.NoError(t, err)

			// Set up our knapsack
			testKnapsack := New(map[storage.Store]types.KVStore{
				storage.ConfigStore:     configStore,
				storage.EnrollmentStore: enrollmentStore,
			}, nil, nil, multislogger.New(), multislogger.New())

			// Set up our registration
			require.NoError(t, testKnapsack.SaveEnrollment(tt.enrollmentId, "test_munemo", tt.expectedNodeKey, ""))

			// Confirm that the node key was stored
			expectedNodeKeyKey := storage.KeyByIdentifier(nodeKeyKey, storage.IdentifierTypeEnrollment, []byte(tt.enrollmentId))
			storedKey, err := configStore.Get(expectedNodeKeyKey)
			require.NoError(t, err)
			require.Equal(t, tt.expectedNodeKey, string(storedKey))

			// Fetch the node key
			nodeKey, err := testKnapsack.NodeKey(tt.enrollmentId)
			require.NoError(t, err)
			require.Equal(t, tt.expectedNodeKey, nodeKey)
		})
	}
}

func TestDeleteEnrollment(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName         string
		expectedEnrollmentId string
		expectedMunemo       string
		expectedNodeKey      string
		expectedEnrollSecret string
	}{
		{
			testCaseName:         "all data set, default enrollment id",
			expectedEnrollmentId: types.DefaultEnrollmentID,
			expectedMunemo:       "test_munemo",
			expectedNodeKey:      "test_node_key",
			expectedEnrollSecret: "test_jwt",
		},
		{
			testCaseName:         "all data set, non-default enrollment id",
			expectedEnrollmentId: ulid.New(),
			expectedMunemo:       "test_munemo",
			expectedNodeKey:      "test_node_key",
			expectedEnrollSecret: "test_jwt",
		},
		{
			testCaseName:         "no enroll secret, default enrollment ID",
			expectedEnrollmentId: types.DefaultEnrollmentID,
			expectedMunemo:       "test_munemo",
			expectedNodeKey:      "test_node_key",
			expectedEnrollSecret: "",
		},
		{
			testCaseName:         "no enroll secret, non-default enrollment ID",
			expectedEnrollmentId: ulid.New(),
			expectedMunemo:       "test_munemo",
			expectedNodeKey:      "test_node_key",
			expectedEnrollSecret: "",
		},
	} {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			// Set up our stores
			configStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String())
			require.NoError(t, err)
			enrollmentStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.EnrollmentStore.String())
			require.NoError(t, err)

			// Set up our knapsack
			testKnapsack := New(map[storage.Store]types.KVStore{
				storage.ConfigStore:     configStore,
				storage.EnrollmentStore: enrollmentStore,
			}, nil, nil, multislogger.New(), multislogger.New())

			// Save the enrollment
			require.NoError(t, testKnapsack.SaveEnrollment(tt.expectedEnrollmentId, tt.expectedMunemo, tt.expectedNodeKey, tt.expectedEnrollSecret))

			// Confirm we have the enrollment
			enrollmentsAfterSave, err := testKnapsack.Enrollments()
			require.NoError(t, err)
			require.Equal(t, 1, len(enrollmentsAfterSave))
			require.Equal(t, tt.expectedEnrollmentId, enrollmentsAfterSave[0].EnrollmentID)

			// Confirm we have the node key
			nodeKey, err := testKnapsack.NodeKey(tt.expectedEnrollmentId)
			require.NoError(t, err)
			require.Equal(t, tt.expectedNodeKey, nodeKey)

			// Now, delete the enrollment
			require.NoError(t, testKnapsack.DeleteEnrollment(tt.expectedEnrollmentId))

			// Confirm the enrollment is gone
			enrollmentsAfterDelete, err := testKnapsack.Enrollments()
			require.NoError(t, err)
			require.Equal(t, 0, len(enrollmentsAfterDelete))

			// Confirm the node key was deleted
			newNodeKey, err := testKnapsack.NodeKey(tt.expectedEnrollmentId)
			require.NoError(t, err)
			require.Equal(t, "", newNodeKey)
		})
	}
}

func TestSetGetEnrollmentDetails(t *testing.T) {
	t.Parallel()

	// Set up the enrollment details store
	enrollmentDetailsStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.EnrollmentDetailsStore.String())
	require.NoError(t, err)

	// Set up our knapsack
	testKnapsack := New(map[storage.Store]types.KVStore{
		storage.EnrollmentDetailsStore: enrollmentDetailsStore,
	}, nil, nil, multislogger.New(), multislogger.New())

	// Initially, should return empty details
	details := testKnapsack.GetEnrollmentDetails()
	require.Equal(t, types.EnrollmentDetails{}, details)

	// Set initial enrollment details
	initialDetails := types.EnrollmentDetails{
		OSPlatform:     "darwin",
		OSVersion:      "10.15.7",
		Hostname:       "test-host",
		OsqueryVersion: "5.0.1",
		HardwareModel:  "MacBookPro16,1",
	}
	testKnapsack.SetEnrollmentDetails(initialDetails)

	// Get and verify
	storedDetails := testKnapsack.GetEnrollmentDetails()
	require.Equal(t, initialDetails.OSPlatform, storedDetails.OSPlatform)
	require.Equal(t, initialDetails.OSVersion, storedDetails.OSVersion)
	require.Equal(t, initialDetails.Hostname, storedDetails.Hostname)
	require.Equal(t, initialDetails.HardwareModel, storedDetails.HardwareModel)

	// Update with partial details (merge behavior)
	updateDetails := types.EnrollmentDetails{
		Hostname:       "new-host",
		OsqueryVersion: "5.0.2",
		GOOS:           "darwin",
	}
	testKnapsack.SetEnrollmentDetails(updateDetails)

	// Get and verify merge worked
	mergedDetails := testKnapsack.GetEnrollmentDetails()
	require.Equal(t, "darwin", mergedDetails.OSPlatform)            // from initial
	require.Equal(t, "10.15.7", mergedDetails.OSVersion)            // from initial
	require.Equal(t, "new-host", mergedDetails.Hostname)            // updated
	require.Equal(t, "MacBookPro16,1", mergedDetails.HardwareModel) // from initial
	require.Equal(t, "darwin", mergedDetails.GOOS)                  // newly added

	// Verify data persists in store
	rawData, err := enrollmentDetailsStore.Get(enrollmentDetailsKey)
	require.NoError(t, err)
	require.NotNil(t, rawData)

	var persistedDetails types.EnrollmentDetails
	require.NoError(t, json.Unmarshal(rawData, &persistedDetails))
	require.Equal(t, "new-host", persistedDetails.Hostname)
	require.Equal(t, "darwin", persistedDetails.OSPlatform)
}

func TestCurrentEnrollmentStatus(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName   string
		isSecretless   bool
		hasNodeKey     bool
		expectedStatus types.EnrollmentStatus
	}{
		{
			testCaseName:   "secretless, unenrolled",
			isSecretless:   true,
			hasNodeKey:     false,
			expectedStatus: types.NoEnrollmentKey,
		},
		{
			testCaseName:   "secretless, enrolled",
			isSecretless:   true,
			hasNodeKey:     true,
			expectedStatus: types.Enrolled,
		},
		{
			testCaseName:   "not secretless, unenrolled",
			isSecretless:   false,
			hasNodeKey:     false,
			expectedStatus: types.Unenrolled,
		},
		{
			testCaseName:   "not secretless, enrolled",
			isSecretless:   false,
			hasNodeKey:     true,
			expectedStatus: types.Enrolled,
		},
	} {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			// Set up our stores
			configStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String())
			require.NoError(t, err)
			enrollmentStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.EnrollmentStore.String())
			require.NoError(t, err)

			// Set up our knapsack
			mockFlags := typesmocks.NewFlags(t)
			testKnapsack := New(map[storage.Store]types.KVStore{
				storage.ConfigStore:     configStore,
				storage.EnrollmentStore: enrollmentStore,
			}, mockFlags, nil, multislogger.New(), multislogger.New())

			testMunemo := ulid.New()
			testEnrollSecret := ""

			if !tt.isSecretless {
				testEnrollSecret = createTestEnrollSecret(t, testMunemo)
			}
			mockFlags.On("EnrollSecret").Return(testEnrollSecret, nil).Maybe()
			mockFlags.On("EnrollSecretPath").Return("").Maybe()

			if tt.hasNodeKey {
				require.NoError(t, testKnapsack.SaveEnrollment(types.DefaultEnrollmentID, testMunemo, ulid.New(), testEnrollSecret))
			}

			gotStatus, err := testKnapsack.CurrentEnrollmentStatus()
			require.NoError(t, err)
			require.Equal(t, tt.expectedStatus, gotStatus)
		})
	}
}

func TestReadEnrollSecret(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName    string
		setViaFlags     bool
		setInFile       bool
		setInTokenStore bool
		secretExpected  bool
	}{
		{
			testCaseName:   "set via command-line args",
			setViaFlags:    true,
			secretExpected: true,
		},
		{
			testCaseName:   "set via secret file",
			setInFile:      true,
			secretExpected: true,
		},
		{
			testCaseName:    "set via launcher enroll subcommand",
			setInTokenStore: true,
			secretExpected:  true,
		},
		{
			testCaseName:   "not set",
			secretExpected: false,
		},
	} {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			// Set up our stores
			tokenStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.TokenStore.String())
			require.NoError(t, err)

			// Set up our knapsack
			mockFlags := typesmocks.NewFlags(t)
			testKnapsack := New(map[storage.Store]types.KVStore{
				storage.TokenStore: tokenStore,
			}, mockFlags, nil, multislogger.New(), multislogger.New())

			// Set up our secret in the indicated location
			testMunemo := ulid.New()
			testEnrollSecret := createTestEnrollSecret(t, testMunemo)

			if tt.setViaFlags {
				mockFlags.On("EnrollSecret").Return(testEnrollSecret)
			} else {
				mockFlags.On("EnrollSecret").Return("").Maybe()
			}

			if tt.setInFile {
				tempEnrollSecretDir := t.TempDir()
				tempEnrollSecretPath := filepath.Join(tempEnrollSecretDir, "secret")
				require.NoError(t, os.WriteFile(tempEnrollSecretPath, []byte(testEnrollSecret), 0755))
				mockFlags.On("EnrollSecretPath").Return(tempEnrollSecretPath)
			} else {
				mockFlags.On("EnrollSecretPath").Return("").Maybe()
			}

			if tt.setInTokenStore {
				require.NoError(t, tokenStore.Set(storage.KeyByIdentifier(storage.EnrollmentSecretTokenKey, storage.IdentifierTypeEnrollment, []byte(types.DefaultEnrollmentID)), []byte(testEnrollSecret)))
			}

			gotSecret, err := testKnapsack.ReadEnrollSecret()
			if tt.secretExpected {
				require.NoError(t, err)
				require.Equal(t, testEnrollSecret, gotSecret)
			} else {
				require.Error(t, err)
			}
		})
	}
}

// createTestEnrollSecret creates a JWT that can be parsed by the knapsack
// to extract its munemo.
func createTestEnrollSecret(t *testing.T, munemo string) string {
	testSigningKey := []byte("test-key")

	type CustomKolideJwtClaims struct {
		Munemo string `json:"organization"`
		jwt.RegisteredClaims
	}

	claims := CustomKolideJwtClaims{
		munemo,
		jwt.RegisteredClaims{
			// A usual scenario is to set the expiration time relative to the current time
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "test",
			Subject:   "somebody",
			ID:        "1",
			Audience:  []string{"somebody_else"},
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedTokenStr, err := token.SignedString(testSigningKey)
	require.NoError(t, err)

	return signedTokenStr
}
