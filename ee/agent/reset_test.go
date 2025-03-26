package agent

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kolide/kit/fsutil"
	"github.com/kolide/krypto/pkg/echelper"
	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	"github.com/kolide/launcher/ee/agent/types"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/packaging"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var testOsqueryBinary string

// TestMain allows us to download osquery once, for use in all tests, instead of
// downloading once per test case.
func TestMain(m *testing.M) {
	downloadDir, err := os.MkdirTemp("", "osquery-runsimple")
	if err != nil {
		fmt.Printf("failed to make temp dir for test osquery binary: %v", err)
		os.Exit(1) //nolint:forbidigo // Fine to use os.Exit inside tests
	}

	target := packaging.Target{}
	if err := target.PlatformFromString(runtime.GOOS); err != nil {
		fmt.Printf("error parsing platform %s: %v", runtime.GOOS, err)
		os.RemoveAll(downloadDir) // explicit removal as defer will not run when os.Exit is called
		os.Exit(1)                //nolint:forbidigo // Fine to use os.Exit inside tests
	}
	target.Arch = packaging.ArchFlavor(runtime.GOARCH)
	if runtime.GOOS == "darwin" {
		target.Arch = packaging.Universal
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	dlPath, err := packaging.FetchBinary(ctx, downloadDir, "osqueryd", target.PlatformBinaryName("osqueryd"), "nightly", target)
	if err != nil {
		fmt.Printf("error fetching binary osqueryd binary: %v", err)
		cancel()                  // explicit cancel as defer will not run when os.Exit is called
		os.RemoveAll(downloadDir) // explicit removal as defer will not run when os.Exit is called
		os.Exit(1)                //nolint:forbidigo // Fine to use os.Exit inside tests
	}

	testOsqueryBinary = filepath.Join(downloadDir, filepath.Base(dlPath))
	if runtime.GOOS == "windows" {
		testOsqueryBinary += ".exe"
	}

	if err := fsutil.CopyFile(dlPath, testOsqueryBinary); err != nil {
		fmt.Printf("error copying osqueryd binary: %v", err)
		cancel()                  // explicit cancel as defer will not run when os.Exit is called
		os.RemoveAll(downloadDir) // explicit removal as defer will not run when os.Exit is called
		os.Exit(1)                //nolint:forbidigo // Fine to use os.Exit inside tests
	}

	// Run the tests
	retCode := m.Run()

	cancel()                  // explicit cancel as defer will not run when os.Exit is called
	os.RemoveAll(downloadDir) // explicit removal as defer will not run when os.Exit is called
	os.Exit(retCode)          //nolint:forbidigo // Fine to use os.Exit inside tests
}

func TestDetectAndRemediateHardwareChange(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                   string
		serialSetInStore       bool
		serialChanged          bool
		hardwareUUIDSetInStore bool
		hardwareUUIDChanged    bool
		osquerySuccess         bool
		munemoSetInStore       bool
		munemoChanged          bool
		secretLivesInFile      bool
		secretReadable         bool
		expectDatabaseWipe     bool
	}{
		{
			name:                   "all data available and unchanged, no database wipe",
			serialSetInStore:       true,
			serialChanged:          false,
			hardwareUUIDSetInStore: true,
			hardwareUUIDChanged:    false,
			osquerySuccess:         true,
			munemoSetInStore:       true,
			munemoChanged:          false,
			secretLivesInFile:      true,
			secretReadable:         true,
			expectDatabaseWipe:     false,
		},
		{
			name:                   "serial changed, database wipe",
			serialSetInStore:       true,
			serialChanged:          true,
			hardwareUUIDSetInStore: true,
			hardwareUUIDChanged:    false,
			osquerySuccess:         true,
			munemoSetInStore:       true,
			munemoChanged:          false,
			secretLivesInFile:      true,
			secretReadable:         true,
			expectDatabaseWipe:     true,
		},
		{
			name:                   "hardware UUID changed, database wipe",
			serialSetInStore:       true,
			serialChanged:          false,
			hardwareUUIDSetInStore: true,
			hardwareUUIDChanged:    true,
			osquerySuccess:         true,
			munemoSetInStore:       true,
			munemoChanged:          false,
			secretLivesInFile:      true,
			secretReadable:         true,
			expectDatabaseWipe:     true,
		},
		{
			name:                   "munemo changed, database wipe",
			serialSetInStore:       true,
			serialChanged:          false,
			hardwareUUIDSetInStore: true,
			hardwareUUIDChanged:    false,
			osquerySuccess:         true,
			munemoSetInStore:       true,
			munemoChanged:          true,
			secretLivesInFile:      true,
			secretReadable:         true,
			expectDatabaseWipe:     true,
		},
		{
			name:                   "munemo changed, enroll secret does not live in file, database wipe",
			serialSetInStore:       true,
			serialChanged:          false,
			hardwareUUIDSetInStore: true,
			hardwareUUIDChanged:    false,
			osquerySuccess:         true,
			munemoSetInStore:       true,
			munemoChanged:          true,
			secretLivesInFile:      false,
			secretReadable:         true,
			expectDatabaseWipe:     true,
		},
		{
			name:                   "multiple values changed, database wipe",
			serialSetInStore:       true,
			serialChanged:          true,
			hardwareUUIDSetInStore: true,
			hardwareUUIDChanged:    true,
			osquerySuccess:         true,
			munemoSetInStore:       true,
			munemoChanged:          true,
			secretLivesInFile:      true,
			secretReadable:         true,
			expectDatabaseWipe:     true,
		},
		{
			name:                   "osquery failed and secret unchanged, no database wipe",
			serialSetInStore:       true,
			serialChanged:          false,
			hardwareUUIDSetInStore: true,
			hardwareUUIDChanged:    false,
			osquerySuccess:         false,
			munemoSetInStore:       true,
			munemoChanged:          false,
			secretLivesInFile:      true,
			secretReadable:         true,
			expectDatabaseWipe:     false,
		},
		{
			name:                   "cannot read secret, no database wipe",
			serialSetInStore:       true,
			serialChanged:          false,
			hardwareUUIDSetInStore: true,
			hardwareUUIDChanged:    false,
			osquerySuccess:         true,
			munemoSetInStore:       true,
			munemoChanged:          false,
			secretLivesInFile:      true,
			secretReadable:         false,
			expectDatabaseWipe:     false,
		},
		{
			name:                   "serial not previously stored, other data unchanged, no database wipe",
			serialSetInStore:       false,
			serialChanged:          false,
			hardwareUUIDSetInStore: true,
			hardwareUUIDChanged:    false,
			osquerySuccess:         true,
			munemoSetInStore:       true,
			munemoChanged:          false,
			secretLivesInFile:      true,
			secretReadable:         true,
			expectDatabaseWipe:     false,
		},
		{
			name:                   "hardware UUID not previously stored, other data unchanged, no database wipe",
			serialSetInStore:       true,
			serialChanged:          false,
			hardwareUUIDSetInStore: false,
			hardwareUUIDChanged:    false,
			osquerySuccess:         true,
			munemoSetInStore:       true,
			munemoChanged:          false,
			secretLivesInFile:      true,
			secretReadable:         true,
			expectDatabaseWipe:     false,
		},
		{
			name:                   "munemo not previously stored, other data unchanged, no database wipe",
			serialSetInStore:       true,
			serialChanged:          false,
			hardwareUUIDSetInStore: true,
			hardwareUUIDChanged:    false,
			osquerySuccess:         true,
			munemoSetInStore:       false,
			munemoChanged:          false,
			secretLivesInFile:      true,
			secretReadable:         true,
			expectDatabaseWipe:     false,
		},
		{
			name:                   "serial not previously stored, other data changed, database wipe",
			serialSetInStore:       false,
			serialChanged:          false,
			hardwareUUIDSetInStore: true,
			hardwareUUIDChanged:    true,
			osquerySuccess:         true,
			munemoSetInStore:       true,
			munemoChanged:          false,
			secretLivesInFile:      true,
			secretReadable:         true,
			expectDatabaseWipe:     true,
		},
	}

	for _, tt := range testCases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// For now, we never expect the database to be wiped. In the future, when we
			// decide to proceed with resetting the database, we can remove this line from
			// the tests and they will continue to validate expected behavior.
			tt.expectDatabaseWipe = false

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

			// Set up logger
			mockKnapsack.On("Slogger").Return(slogger)

			// Set up dependencies: ensure that retrieved hardware data matches expectations
			var actualSerial, actualHardwareUUID string
			if tt.osquerySuccess {
				mockKnapsack.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinary)
				actualSerial, actualHardwareUUID, err = currentSerialAndHardwareUUID(context.TODO(), mockKnapsack)
				require.NoError(t, err, "expected no error querying osquery at ", testOsqueryBinary)
			} else {
				mockKnapsack.On("LatestOsquerydPath", mock.Anything).Return(filepath.Join("not", "a", "real", "osqueryd", "binary"))
				actualSerial = "test-serial"
				actualHardwareUUID = "test-hardware-uuid"
			}

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

			// Set up dependencies: ensure that retrieved tenant matches current data
			munemoValue := []byte("test-munemo")
			secretJwt := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"organization": string(munemoValue)})
			secretValue, err := secretJwt.SignedString(jwt.UnsafeAllowNoneSignatureType)
			require.NoError(t, err, "could not create test enroll secret")

			if tt.secretLivesInFile {
				var secretFilepath string
				if tt.secretReadable {
					secretDir := t.TempDir()
					secretFilepath = filepath.Join(secretDir, "test-secret")
					require.NoError(t, os.WriteFile(secretFilepath, []byte(secretValue), 0644), "could not write out test secret")
				} else {
					secretFilepath = filepath.Join("not", "a", "real", "enroll", "secret")
				}

				mockKnapsack.On("EnrollSecret").Return("")
				mockKnapsack.On("EnrollSecretPath").Return(secretFilepath)
			} else {
				mockKnapsack.On("EnrollSecret").Return(secretValue)
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
			DetectAndRemediateHardwareChange(context.TODO(), mockKnapsack)

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
			if tt.secretReadable {
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

	t.Skip("un-skip test once we decide to reset the database on hardware change")

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

	// Set up logger
	mockKnapsack.On("Slogger").Return(slogger)

	// Set up dependencies: ensure that retrieved hardware data matches expectations
	var actualSerial, actualHardwareUUID string
	mockKnapsack.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinary)
	actualSerial, actualHardwareUUID, err = currentSerialAndHardwareUUID(context.TODO(), mockKnapsack)
	require.NoError(t, err, "expected no error querying osquery at ", testOsqueryBinary)
	require.NoError(t, testHostDataStore.Set(hostDataKeySerial, []byte(actualSerial)), "could not set serial in test store")
	require.NoError(t, testHostDataStore.Set(hostDataKeyHardwareUuid, []byte(actualHardwareUUID)), "could not set hardware uuid in test store")

	// Set up dependencies: ensure that retrieved tenant has changed from test-munemo-1 (stored)
	// to test-munemo-2 (new file)
	firstMunemoValue := []byte("test-munemo-1")
	secondMunemoValue := []byte("test-munemo-2")
	secretJwt := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"organization": string(secondMunemoValue)})
	secretValue, err := secretJwt.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err, "could not create test enroll secret")

	secretDir := t.TempDir()
	secretFilepath := filepath.Join(secretDir, "test-secret")
	require.NoError(t, os.WriteFile(secretFilepath, []byte(secretValue), 0644), "could not write out test secret")
	mockKnapsack.On("EnrollSecret").Return("")
	mockKnapsack.On("EnrollSecretPath").Return(secretFilepath)
	require.NoError(t, testHostDataStore.Set(hostDataKeyMunemo, firstMunemoValue), "could not set munemo in test store")

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
	DetectAndRemediateHardwareChange(context.TODO(), mockKnapsack)

	// Confirm the old_host_data key exists in the data store
	dataRaw, err := testHostDataStore.Get(hostDataKeyResetRecords)
	require.NoError(t, err, "could not get old host data from test store")
	require.NotNil(t, dataRaw, "old host data not set in store")

	// Confirm that it contains reasonable data: we should have one backup
	// with the first munemo in it
	var d []dbResetRecord
	require.NoError(t, json.Unmarshal(dataRaw, &d), "old host data in unexpected format")
	require.Equal(t, 1, len(d), "unexpected number of backups")
	require.Equal(t, string(firstMunemoValue), d[0].Munemo, "munemo does not match")

	// The current saved munemo should equal the second munemo
	munemo, err := testHostDataStore.Get(hostDataKeyMunemo)
	require.NoError(t, err, "could not get munemo from test store")
	require.Equal(t, secondMunemoValue, munemo, "munemo in test store does not match expected munemo")

	// Now, perform secret setup again, setting the munemo to a new third value.
	thirdMunemoValue := []byte("test-munemo-3")
	newJwt := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"organization": string(thirdMunemoValue)})
	newSecretValue, err := newJwt.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err, "could not create test enroll secret")
	require.NoError(t, os.WriteFile(secretFilepath, []byte(newSecretValue), 0644), "could not write out test secret")

	// Set backup data again
	require.NoError(t, testConfigStore.Set([]byte("nodeKey"), []byte(testNodeKey)), "could not set value in test store")
	require.NoError(t, testServerProvidedDataStore.Set([]byte("device_id"), []byte(testDeviceId)), "could not set value in test store")
	require.NoError(t, testServerProvidedDataStore.Set([]byte("remote_ip"), []byte(testRemoteIp)), "could not set value in test store")
	require.NoError(t, testServerProvidedDataStore.Set([]byte("tombstone_id"), []byte(testTombstoneId)), "could not set value in test store")
	require.NoError(t, testConfigStore.Set([]byte("localEccKey"), testLocalEccKeyRaw))

	// Make second test call
	DetectAndRemediateHardwareChange(context.TODO(), mockKnapsack)

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
	require.Equal(t, string(firstMunemoValue), dNew[0].Munemo, "first backup munemo does not match")
	require.Equal(t, string(secondMunemoValue), dNew[1].Munemo, "second backup munemo does not match")

	// The current saved munemo should equal the third
	currentMunemo, err := testHostDataStore.Get(hostDataKeyMunemo)
	require.NoError(t, err, "could not get munemo from test store")
	require.Equal(t, thirdMunemoValue, currentMunemo, "munemo in test store does not match expected munemo")

	// Make sure all the functions were called as expected
	mockKnapsack.AssertExpectations(t)
}
