package agent

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	"github.com/kolide/launcher/ee/agent/types"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/osquery/testutil"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var testOsqueryBinary string

// downloadOnceFunc downloads a real osquery binary for use in tests. This function
// can be called multiple times but will only execute once -- the osquery binary is
// stored at path `testOsqueryBinary` and can be reused by all subsequent tests.
var downloadOnceFunc = sync.OnceFunc(func() {
	testOsqueryBinary, _, _ = testutil.DownloadOsquery("nightly")
})

func TestDetectAndRemediateHardwareChange(t *testing.T) {
	t.Parallel()
	downloadOnceFunc()

	testCases := []struct {
		name                         string
		serialSetInStore             bool
		serialChanged                bool
		hardwareUUIDSetInStore       bool
		hardwareUUIDChanged          bool
		machineGUIDSetInStore        bool
		machineGUIDChanged           bool
		osquerySuccess               bool
		munemoSetInStore             bool
		munemoChanged                bool
		registrationsExist           bool
		resetOnHardwareChangeEnabled bool
		expectDatabaseWipe           bool
	}{
		{
			name:                         "all data available and unchanged, no database wipe",
			serialSetInStore:             true,
			serialChanged:                false,
			hardwareUUIDSetInStore:       true,
			hardwareUUIDChanged:          false,
			machineGUIDSetInStore:        false,
			machineGUIDChanged:           false,
			osquerySuccess:               true,
			munemoSetInStore:             true,
			munemoChanged:                false,
			registrationsExist:           true,
			resetOnHardwareChangeEnabled: true,
			expectDatabaseWipe:           false,
		},
		{
			name:                         "serial changed, no database wipe",
			serialSetInStore:             true,
			serialChanged:                true,
			hardwareUUIDSetInStore:       true,
			hardwareUUIDChanged:          false,
			machineGUIDSetInStore:        false,
			machineGUIDChanged:           false,
			osquerySuccess:               true,
			munemoSetInStore:             true,
			munemoChanged:                false,
			registrationsExist:           true,
			resetOnHardwareChangeEnabled: true,
			expectDatabaseWipe:           false,
		},
		{
			name:                         "hardware UUID changed, no database wipe",
			serialSetInStore:             true,
			serialChanged:                false,
			hardwareUUIDSetInStore:       true,
			hardwareUUIDChanged:          true,
			machineGUIDSetInStore:        false,
			machineGUIDChanged:           false,
			osquerySuccess:               true,
			munemoSetInStore:             true,
			munemoChanged:                false,
			registrationsExist:           true,
			resetOnHardwareChangeEnabled: true,
			expectDatabaseWipe:           false,
		},
		{
			name:                         "munemo changed, no database wipe",
			serialSetInStore:             true,
			serialChanged:                false,
			hardwareUUIDSetInStore:       true,
			hardwareUUIDChanged:          false,
			machineGUIDSetInStore:        false,
			machineGUIDChanged:           false,
			osquerySuccess:               true,
			munemoSetInStore:             true,
			munemoChanged:                true,
			registrationsExist:           true,
			resetOnHardwareChangeEnabled: true,
			expectDatabaseWipe:           false,
		},
		{
			name:                         "multiple values changed, feature flag enabled, database wipe",
			serialSetInStore:             true,
			serialChanged:                true,
			hardwareUUIDSetInStore:       true,
			hardwareUUIDChanged:          true,
			machineGUIDSetInStore:        true,
			machineGUIDChanged:           true,
			osquerySuccess:               true,
			munemoSetInStore:             true,
			munemoChanged:                true,
			registrationsExist:           true,
			resetOnHardwareChangeEnabled: true,
			expectDatabaseWipe:           true,
		},
		{
			name:                         "multiple values changed, feature flag not enabled, no database wipe",
			serialSetInStore:             true,
			serialChanged:                true,
			hardwareUUIDSetInStore:       true,
			hardwareUUIDChanged:          true,
			machineGUIDSetInStore:        true,
			machineGUIDChanged:           true,
			osquerySuccess:               true,
			munemoSetInStore:             true,
			munemoChanged:                true,
			registrationsExist:           true,
			resetOnHardwareChangeEnabled: false,
			expectDatabaseWipe:           false,
		},
		{
			name:                         "hardware and serial changed, feature flag enabled, database wipe on non-Windows only",
			serialSetInStore:             true,
			serialChanged:                true,
			hardwareUUIDSetInStore:       true,
			hardwareUUIDChanged:          true,
			machineGUIDSetInStore:        true,
			machineGUIDChanged:           false,
			osquerySuccess:               true,
			munemoSetInStore:             true,
			munemoChanged:                true,
			registrationsExist:           true,
			resetOnHardwareChangeEnabled: true,
			expectDatabaseWipe:           runtime.GOOS != "windows",
		},
		{
			name:                         "osquery failed and secret unchanged, no database wipe",
			serialSetInStore:             true,
			serialChanged:                false,
			hardwareUUIDSetInStore:       true,
			hardwareUUIDChanged:          false,
			machineGUIDSetInStore:        false,
			machineGUIDChanged:           false,
			osquerySuccess:               false,
			munemoSetInStore:             true,
			munemoChanged:                false,
			registrationsExist:           true,
			resetOnHardwareChangeEnabled: true,
			expectDatabaseWipe:           false,
		},
		{
			name:                         "cannot read secret, no database wipe",
			serialSetInStore:             true,
			serialChanged:                false,
			hardwareUUIDSetInStore:       true,
			hardwareUUIDChanged:          false,
			machineGUIDSetInStore:        false,
			machineGUIDChanged:           false,
			osquerySuccess:               true,
			munemoSetInStore:             true,
			munemoChanged:                false,
			registrationsExist:           false,
			resetOnHardwareChangeEnabled: true,
			expectDatabaseWipe:           false,
		},
		{
			name:                         "serial not previously stored, other data unchanged, no database wipe",
			serialSetInStore:             false,
			serialChanged:                false,
			hardwareUUIDSetInStore:       true,
			hardwareUUIDChanged:          false,
			machineGUIDSetInStore:        false,
			machineGUIDChanged:           false,
			osquerySuccess:               true,
			munemoSetInStore:             true,
			munemoChanged:                false,
			registrationsExist:           true,
			resetOnHardwareChangeEnabled: true,
			expectDatabaseWipe:           false,
		},
		{
			name:                         "hardware UUID not previously stored, other data unchanged, no database wipe",
			serialSetInStore:             true,
			serialChanged:                false,
			hardwareUUIDSetInStore:       false,
			hardwareUUIDChanged:          false,
			machineGUIDSetInStore:        false,
			machineGUIDChanged:           false,
			osquerySuccess:               true,
			munemoSetInStore:             true,
			munemoChanged:                false,
			registrationsExist:           true,
			resetOnHardwareChangeEnabled: true,
			expectDatabaseWipe:           false,
		},
		{
			name:                         "munemo not previously stored, other data unchanged, no database wipe",
			serialSetInStore:             true,
			serialChanged:                false,
			hardwareUUIDSetInStore:       true,
			hardwareUUIDChanged:          false,
			machineGUIDSetInStore:        false,
			machineGUIDChanged:           false,
			osquerySuccess:               true,
			munemoSetInStore:             false,
			munemoChanged:                false,
			registrationsExist:           true,
			resetOnHardwareChangeEnabled: true,
			expectDatabaseWipe:           false,
		},
		{
			name:                         "serial not previously stored, other data changed, no database wipe",
			serialSetInStore:             false,
			serialChanged:                false,
			hardwareUUIDSetInStore:       true,
			hardwareUUIDChanged:          true,
			machineGUIDSetInStore:        false,
			machineGUIDChanged:           false,
			osquerySuccess:               true,
			munemoSetInStore:             true,
			munemoChanged:                false,
			registrationsExist:           true,
			resetOnHardwareChangeEnabled: true,
			expectDatabaseWipe:           false,
		},
		{
			name:                         "machine GUID previously stored, all other data unchanged, no database wipe",
			serialSetInStore:             true,
			serialChanged:                false,
			hardwareUUIDSetInStore:       true,
			hardwareUUIDChanged:          false,
			machineGUIDSetInStore:        true,
			machineGUIDChanged:           false,
			osquerySuccess:               true,
			munemoSetInStore:             true,
			munemoChanged:                false,
			registrationsExist:           true,
			resetOnHardwareChangeEnabled: true,
			expectDatabaseWipe:           false,
		},
		{
			name:                         "machine GUID not previously stored, all other data unchanged, no database wipe",
			serialSetInStore:             true,
			serialChanged:                false,
			hardwareUUIDSetInStore:       true,
			hardwareUUIDChanged:          false,
			machineGUIDSetInStore:        false,
			machineGUIDChanged:           true,
			osquerySuccess:               true,
			munemoSetInStore:             true,
			munemoChanged:                false,
			registrationsExist:           true,
			resetOnHardwareChangeEnabled: true,
			expectDatabaseWipe:           false,
		},
		{
			name:                         "machine GUID previously stored, then changed, feature flag enabled, expect database wipe on Windows only",
			serialSetInStore:             true,
			serialChanged:                false,
			hardwareUUIDSetInStore:       true,
			hardwareUUIDChanged:          false,
			machineGUIDSetInStore:        true,
			machineGUIDChanged:           true,
			osquerySuccess:               true,
			munemoSetInStore:             true,
			munemoChanged:                false,
			registrationsExist:           true,
			resetOnHardwareChangeEnabled: true,
			expectDatabaseWipe:           runtime.GOOS == "windows",
		},
		{
			name:                         "machine GUID previously stored, then changed, feature flag not enabled, no database wipe",
			serialSetInStore:             true,
			serialChanged:                false,
			hardwareUUIDSetInStore:       true,
			hardwareUUIDChanged:          false,
			machineGUIDSetInStore:        true,
			machineGUIDChanged:           true,
			osquerySuccess:               true,
			munemoSetInStore:             true,
			munemoChanged:                false,
			registrationsExist:           true,
			resetOnHardwareChangeEnabled: false,
			expectDatabaseWipe:           false,
		},
	}

	for _, tt := range testCases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			slogger := multislogger.NewNopLogger()

			// Set up dependencies: data store for hardware-identifying data
			testHostDataStore, err := storageci.NewStore(t, slogger, storage.PersistentHostDataStore.String())
			require.NoError(t, err, "could not create test host data store")
			mockKnapsack := typesmocks.NewKnapsack(t)
			mockKnapsack.On("PersistentHostDataStore").Return(testHostDataStore)
			testConfigStore, err := storageci.NewStore(t, slogger, storage.ConfigStore.String())
			require.NoError(t, err, "could not create test config store")
			mockKnapsack.On("ConfigStore").Return(testConfigStore).Maybe()
			testServerProvidedDataStore, err := storageci.NewStore(t, slogger, storage.ServerProvidedDataStore.String())
			require.NoError(t, err, "could not create test server provided data store")
			mockKnapsack.On("ServerProvidedDataStore").Return(testServerProvidedDataStore).Maybe()
			mockKnapsack.On("Stores").Return(map[storage.Store]types.KVStore{
				storage.PersistentHostDataStore: testHostDataStore,
				storage.ConfigStore:             testConfigStore,
				storage.ServerProvidedDataStore: testServerProvidedDataStore,
			}).Maybe()
			mockKnapsack.On("ResetOnHardwareChangeEnabled").Return(tt.resetOnHardwareChangeEnabled).Maybe()
			mockKnapsack.On("RegistrationIDs").Return([]string{"default"}).Maybe()

			// Set up dependencies: ensure that retrieved hardware data matches expectations
			var actualSerial, actualHardwareUUID string
			if tt.osquerySuccess {
				mockKnapsack.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinary)
				actualSerial, actualHardwareUUID, err = currentSerialAndHardwareUUID(t.Context(), mockKnapsack)
				require.NoError(t, err, "expected no error querying osquery at ", testOsqueryBinary)
			} else {
				mockKnapsack.On("LatestOsquerydPath", mock.Anything).Return(filepath.Join("not", "a", "real", "osqueryd", "binary"))
				actualSerial = "test-serial"
				actualHardwareUUID = "test-hardware-uuid"
			}

			actualMachineGUID, err := currentMachineGuid(t.Context(), mockKnapsack)
			require.NoError(t, err, "expected to be able to read machine GUID")

			if tt.serialSetInStore {
				if tt.serialChanged {
					require.NoError(t, testHostDataStore.Set(hostDataKeySerial, []byte("some-old-serial")), "could not set serial in test store")
				} else {
					require.NoError(t, testHostDataStore.Set(hostDataKeySerial, []byte(actualSerial)), "could not set serial in test store")
				}
			}

			if tt.hardwareUUIDSetInStore {
				if tt.hardwareUUIDChanged {
					require.NoError(t, testHostDataStore.Set(hostDataKeyHardwareUuid, []byte("some-old-hardware-uuid")), "could not set hardware uuid in test store")
				} else {
					require.NoError(t, testHostDataStore.Set(hostDataKeyHardwareUuid, []byte(actualHardwareUUID)), "could not set hardware uuid in test store")
				}
			}

			if tt.machineGUIDSetInStore {
				if tt.machineGUIDChanged {
					require.NoError(t, testHostDataStore.Set(hostDataKeyMachineGuid, []byte("some-old-machine-guid")), "could not set machine guid in test store")
				} else {
					require.NoError(t, testHostDataStore.Set(hostDataKeyMachineGuid, []byte(actualMachineGUID)), "could not set machine guid in test store")
				}
			}

			// Set up dependencies: ensure that retrieved tenant matches current data
			munemoValue := []byte("test-munemo")

			if tt.registrationsExist {
				mockKnapsack.On("Registrations").Return([]types.Registration{
					{
						RegistrationID: types.DefaultRegistrationID,
						Munemo:         string(munemoValue),
					},
				}, nil)
			} else {
				mockKnapsack.On("Registrations").Return(nil, nil)
			}

			if tt.munemoSetInStore {
				if tt.munemoChanged {
					require.NoError(t, testHostDataStore.Set(hostDataKeyMunemo, []byte("some-old-munemo")), "could not set munemo in test store")
				} else {
					require.NoError(t, testHostDataStore.Set(hostDataKeyMunemo, munemoValue), "could not set munemo in test store")
				}
			}

			// Set up dependencies: set an extra key that we know should be deleted when the database
			// is wiped
			extraKey := []byte("test_key")
			extraValue := []byte("this is a test value")
			require.NoError(t, testHostDataStore.Set(extraKey, extraValue), "could not set value in test store")

			// Set up dependencies: set data that we expect to be backed up
			testNodeKey := "abcd"
			require.NoError(t, testConfigStore.Set([]byte("nodeKey"), []byte(testNodeKey)), "could not set value in test store")
			testDeviceId := "1"
			require.NoError(t, testServerProvidedDataStore.Set([]byte("device_id"), []byte(testDeviceId)), "could not set value in test store")
			testRemoteIp := "127.0.0.1"
			require.NoError(t, testServerProvidedDataStore.Set([]byte("remote_ip"), []byte(testRemoteIp)), "could not set value in test store")
			testTombstoneId := "100"
			require.NoError(t, testServerProvidedDataStore.Set([]byte("tombstone_id"), []byte(testTombstoneId)), "could not set value in test store")

			testLocalEccKey, err := echelper.GenerateEcdsaKey()
			require.NoError(t, err, "generating test key for backup")
			testLocalEccKeyRaw, err := x509.MarshalECPrivateKey(testLocalEccKey)
			require.NoError(t, err, "marshalling test key")
			require.NoError(t, testConfigStore.Set([]byte("localEccKey"), testLocalEccKeyRaw))

			// Make test call
			remediationOccurred := detectAndRemediateHardwareChange(t.Context(), mockKnapsack, slogger)
			require.Equal(t, tt.expectDatabaseWipe, remediationOccurred, "expected remediation to occur when database should be wiped")

			// Confirm backup occurred, if database got wiped
			if tt.expectDatabaseWipe {
				// Confirm the old_host_data key exists in the data store
				dataRaw, err := testHostDataStore.Get(hostDataKeyResetRecords)
				require.NoError(t, err, "could not get old host data from test store")
				require.NotNil(t, dataRaw, "old host data not set in store")

				// Confirm that it contains reasonable data
				var d []dbResetRecord
				require.NoError(t, json.Unmarshal(dataRaw, &d), "old host data in unexpected format")

				// We should only have one backup
				require.Equal(t, 1, len(d), "unexpected number of backups")

				// The backup data should be correct
				require.Equal(t, testNodeKey, d[0].NodeKey, "node key does not match")
				require.Equal(t, testDeviceId, d[0].DeviceID, "device id does not match")
				require.Equal(t, testRemoteIp, d[0].RemoteIP, "remote ip does not match")
				require.Equal(t, testTombstoneId, d[0].TombstoneID, "tombstone id does not match")

				// The pubkey should match the test pubkey
				require.Equal(t, 1, len(d[0].PubKeys))
				p, err := x509.ParsePKIXPublicKey(d[0].PubKeys[0])
				require.NoError(t, err, "could not parse stored pubkey")
				eccPubKey, ok := p.(*ecdsa.PublicKey)
				require.True(t, ok, "unexpected pubkey format", fmt.Sprintf("%T", p))
				require.True(t, eccPubKey.Equal(testLocalEccKey.Public()), "pubkey mismatch")

				// The backup timestamp should be in the past
				require.GreaterOrEqual(t, time.Now().Unix(), d[0].ResetTimestamp)

				// The backup timestamp should be at least kind of close to now -- within the last
				// five minutes. Not checking too closely to avoid flaky tests.
				require.LessOrEqual(t, time.Now().Unix()-300, d[0].ResetTimestamp)

				// The reset reason should be correct
				require.Equal(t, resetReasonNewHardwareOrEnrollmentDetected, d[0].ResetReason)
			}

			// Confirm whether the database got wiped
			v, err := testHostDataStore.Get(extraKey)
			require.NoError(t, err)
			if tt.expectDatabaseWipe {
				require.Nil(t, v, "database not wiped")
			} else {
				require.Equal(t, extraValue, v, "database wiped")
			}

			// Confirm hardware-identifying data got set regardless of db wipe,
			// as long as it was available
			if tt.osquerySuccess {
				serial, err := testHostDataStore.Get(hostDataKeySerial)
				require.NoError(t, err, "could not get serial from test store")
				require.Equal(t, actualSerial, string(serial), "serial in test store does not match expected serial")
				hardwareUUID, err := testHostDataStore.Get(hostDataKeyHardwareUuid)
				require.NoError(t, err, "could not get hardware UUID from test store")
				require.Equal(t, actualHardwareUUID, string(hardwareUUID), "hardware UUID in test store does not match expected hardware UUID")
			}
			if tt.registrationsExist {
				munemo, err := testHostDataStore.Get(hostDataKeyMunemo)
				require.NoError(t, err, "could not get munemo from test store")
				require.Equal(t, munemoValue, munemo, "munemo in test store does not match expected munemo")
			}

			// Make sure all the functions were called as expected
			mockKnapsack.AssertExpectations(t)
		})
	}
}

func TestDetectAndRemediateHardwareChange_SavesDataOverMultipleResets(t *testing.T) {
	t.Parallel()
	downloadOnceFunc()

	slogger := multislogger.NewNopLogger()

	// Set up dependencies: data store for hardware-identifying data
	testHostDataStore, err := storageci.NewStore(t, slogger, storage.PersistentHostDataStore.String())
	require.NoError(t, err, "could not create test host data store")
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("PersistentHostDataStore").Return(testHostDataStore)
	testConfigStore, err := storageci.NewStore(t, slogger, storage.ConfigStore.String())
	require.NoError(t, err, "could not create test config store")
	mockKnapsack.On("ConfigStore").Return(testConfigStore)
	testServerProvidedDataStore, err := storageci.NewStore(t, slogger, storage.ServerProvidedDataStore.String())
	require.NoError(t, err, "could not create test server provided data store")
	mockKnapsack.On("ServerProvidedDataStore").Return(testServerProvidedDataStore)
	mockKnapsack.On("Stores").Return(map[storage.Store]types.KVStore{
		storage.PersistentHostDataStore: testHostDataStore,
		storage.ConfigStore:             testConfigStore,
		storage.ServerProvidedDataStore: testServerProvidedDataStore,
	})
	mockKnapsack.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinary)
	mockKnapsack.On("ResetOnHardwareChangeEnabled").Return(true)
	mockKnapsack.On("Registrations").Return([]types.Registration{
		{
			RegistrationID: types.DefaultRegistrationID,
			Munemo:         "test-munemo-1",
		},
	}, nil)
	mockKnapsack.On("RegistrationIDs").Return([]string{"default"})

	// Set up dependencies: ensure that all hardware data is incorrect so that a reset will be triggered
	require.NoError(t, testHostDataStore.Set(hostDataKeySerial, []byte("not-the-correct-serial")), "could not set serial in test store")
	require.NoError(t, testHostDataStore.Set(hostDataKeyHardwareUuid, []byte("not-the-correct-hardware-uuid")), "could not set hardware uuid in test store")
	require.NoError(t, testHostDataStore.Set(hostDataKeyMachineGuid, []byte("not-the-correct-machine-guid")), "could not set machine guid in test store")

	// Set up dependencies: set data that we expect to be backed up
	testNodeKey := "abcd"
	require.NoError(t, testConfigStore.Set([]byte("nodeKey"), []byte(testNodeKey)), "could not set value in test store")
	testDeviceId := "1"
	require.NoError(t, testServerProvidedDataStore.Set([]byte("device_id"), []byte(testDeviceId)), "could not set value in test store")
	testRemoteIp := "127.0.0.1"
	require.NoError(t, testServerProvidedDataStore.Set([]byte("remote_ip"), []byte(testRemoteIp)), "could not set value in test store")
	testTombstoneId := "100"
	require.NoError(t, testServerProvidedDataStore.Set([]byte("tombstone_id"), []byte(testTombstoneId)), "could not set value in test store")

	testLocalEccKey, err := echelper.GenerateEcdsaKey()
	require.NoError(t, err, "generating test key for backup")
	testLocalEccKeyRaw, err := x509.MarshalECPrivateKey(testLocalEccKey)
	require.NoError(t, err, "marshalling test key")
	require.NoError(t, testConfigStore.Set([]byte("localEccKey"), testLocalEccKeyRaw))

	// Make first test call
	require.True(t, detectAndRemediateHardwareChange(t.Context(), mockKnapsack, slogger))

	// Confirm the old_host_data key exists in the data store
	dataRaw, err := testHostDataStore.Get(hostDataKeyResetRecords)
	require.NoError(t, err, "could not get old host data from test store")
	require.NotNil(t, dataRaw, "old host data not set in store")

	// Confirm that it contains reasonable data: we should have one backup
	var d []dbResetRecord
	require.NoError(t, json.Unmarshal(dataRaw, &d), "old host data in unexpected format")
	require.Equal(t, 1, len(d), "unexpected number of backups")

	// Now, reset the hardware data back to incorrect values so that we'll trigger a reset again
	require.NoError(t, testHostDataStore.Set(hostDataKeySerial, []byte("not-the-correct-serial-again")), "could not set serial in test store")
	require.NoError(t, testHostDataStore.Set(hostDataKeyHardwareUuid, []byte("not-the-correct-hardware-uuid-again")), "could not set hardware uuid in test store")
	require.NoError(t, testHostDataStore.Set(hostDataKeyMachineGuid, []byte("not-the-correct-machine-guid-again")), "could not set machine guid in test store")

	// Set backup data again
	require.NoError(t, testConfigStore.Set([]byte("nodeKey"), []byte(testNodeKey)), "could not set value in test store")
	require.NoError(t, testServerProvidedDataStore.Set([]byte("device_id"), []byte(testDeviceId)), "could not set value in test store")
	require.NoError(t, testServerProvidedDataStore.Set([]byte("remote_ip"), []byte(testRemoteIp)), "could not set value in test store")
	require.NoError(t, testServerProvidedDataStore.Set([]byte("tombstone_id"), []byte(testTombstoneId)), "could not set value in test store")
	require.NoError(t, testConfigStore.Set([]byte("localEccKey"), testLocalEccKeyRaw))

	// Make second test call
	require.True(t, detectAndRemediateHardwareChange(t.Context(), mockKnapsack, slogger))

	// Confirm the old_host_data key exists in the data store
	newDataRaw, err := testHostDataStore.Get(hostDataKeyResetRecords)
	require.NoError(t, err, "could not get old host data from test store")
	require.NotNil(t, dataRaw, "old host data not set in store")

	// Confirm that it contains reasonable data: we should have two backups
	// now -- the first should have the first munemo in it, and the second
	// should have the second.
	var dNew []dbResetRecord
	require.NoError(t, json.Unmarshal(newDataRaw, &dNew), "old host data in unexpected format")
	require.Equal(t, 2, len(dNew), "unexpected number of backups")

	// Make sure all the functions were called as expected
	mockKnapsack.AssertExpectations(t)
}

func TestExecute(t *testing.T) {
	t.Parallel()
	downloadOnceFunc()

	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Set up dependencies: data store for hardware-identifying data
	testHostDataStore, err := storageci.NewStore(t, slogger, storage.PersistentHostDataStore.String())
	require.NoError(t, err, "could not create test host data store")
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("PersistentHostDataStore").Return(testHostDataStore)
	testConfigStore, err := storageci.NewStore(t, slogger, storage.ConfigStore.String())
	require.NoError(t, err, "could not create test config store")
	mockKnapsack.On("ConfigStore").Return(testConfigStore)
	testServerProvidedDataStore, err := storageci.NewStore(t, slogger, storage.ServerProvidedDataStore.String())
	require.NoError(t, err, "could not create test server provided data store")
	mockKnapsack.On("ServerProvidedDataStore").Return(testServerProvidedDataStore)
	mockKnapsack.On("Stores").Return(map[storage.Store]types.KVStore{
		storage.PersistentHostDataStore: testHostDataStore,
		storage.ConfigStore:             testConfigStore,
		storage.ServerProvidedDataStore: testServerProvidedDataStore,
	})
	mockKnapsack.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinary)
	mockKnapsack.On("ResetOnHardwareChangeEnabled").Return(true)
	mockKnapsack.On("Registrations").Return([]types.Registration{
		{
			RegistrationID: types.DefaultRegistrationID,
			Munemo:         "test-munemo-1",
		},
	}, nil)
	mockKnapsack.On("RegistrationIDs").Return([]string{"default"})

	// Set up dependencies: ensure that all hardware data is incorrect so that a reset will be triggered
	require.NoError(t, testHostDataStore.Set(hostDataKeySerial, []byte("not-the-correct-serial")), "could not set serial in test store")
	require.NoError(t, testHostDataStore.Set(hostDataKeyHardwareUuid, []byte("not-the-correct-hardware-uuid")), "could not set hardware uuid in test store")
	require.NoError(t, testHostDataStore.Set(hostDataKeyMachineGuid, []byte("not-the-correct-machine-guid")), "could not set machine guid in test store")

	detector := NewHardwareChangeDetector(mockKnapsack, slogger)

	executeErrs := make(chan error)
	go func() {
		executeErrs <- detector.Execute()
	}()

	select {
	case executeErr := <-executeErrs:
		require.True(t, errors.Is(executeErr, ErrNewHardwareDetected), "unexpected error returned from Execute")
	case <-time.After(15 * time.Second):
		t.Errorf("detector did not detect hardware change and return within 15 seconds -- logs: \n%s\n", logBytes.String())
		t.FailNow()
	}
}

func TestInterrupt_Multiple(t *testing.T) {
	t.Parallel()
	downloadOnceFunc()

	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinary)
	testHostDataStore, err := storageci.NewStore(t, slogger, storage.PersistentHostDataStore.String())
	require.NoError(t, err, "could not create test host data store")
	mockKnapsack.On("PersistentHostDataStore").Return(testHostDataStore)
	mockKnapsack.On("Registrations").Return([]types.Registration{
		{
			RegistrationID: types.DefaultRegistrationID,
			Munemo:         "test-munemo",
		},
	}, nil)

	detector := NewHardwareChangeDetector(mockKnapsack, slogger)

	// Start and then interrupt
	go detector.Execute()
	time.Sleep(3 * time.Second)
	interruptStart := time.Now()
	detector.Interrupt(errors.New("test error"))

	// Confirm we can call Interrupt multiple times without blocking
	interruptComplete := make(chan struct{})
	expectedInterrupts := 3
	for i := 0; i < expectedInterrupts; i += 1 {
		go func() {
			detector.Interrupt(nil)
			interruptComplete <- struct{}{}
		}()
	}

	receivedInterrupts := 0
	for {
		if receivedInterrupts >= expectedInterrupts {
			break
		}

		select {
		case <-interruptComplete:
			receivedInterrupts += 1
			continue
		case <-time.After(5 * time.Second):
			t.Errorf("could not call interrupt multiple times and return within 5 seconds -- interrupted at %s, received %d interrupts before timeout; logs: \n%s\n", interruptStart.String(), receivedInterrupts, logBytes.String())
			t.FailNow()
		}
	}

	require.Equal(t, expectedInterrupts, receivedInterrupts)
}
