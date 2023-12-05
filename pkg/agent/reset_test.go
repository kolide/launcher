package agent

import (
	"archive/zip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/fsutil"
	"github.com/kolide/launcher/pkg/agent/storage"
	storageci "github.com/kolide/launcher/pkg/agent/storage/ci"
	"github.com/kolide/launcher/pkg/agent/types"
	typesmocks "github.com/kolide/launcher/pkg/agent/types/mocks"
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
		os.Exit(1)
	}
	defer os.RemoveAll(downloadDir)

	target := packaging.Target{}
	if err := target.PlatformFromString(runtime.GOOS); err != nil {
		fmt.Printf("error parsing platform %s: %v", runtime.GOOS, err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dlPath, err := packaging.FetchBinary(ctx, downloadDir, "osqueryd", target.PlatformBinaryName("osqueryd"), "nightly", target)
	if err != nil {
		fmt.Printf("error fetching binary osqueryd binary: %v", err)
		os.Exit(1)
	}

	testOsqueryBinary = filepath.Join(downloadDir, filepath.Base(dlPath))
	if runtime.GOOS == "windows" {
		testOsqueryBinary += ".exe"
	}

	if err := fsutil.CopyFile(dlPath, testOsqueryBinary); err != nil {
		fmt.Printf("error copying osqueryd binary: %v", err)
		os.Exit(1)
	}

	// Run the tests
	retCode := m.Run()
	os.Exit(retCode)
}

func TestResetDatabaseIfNeeded(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                   string
		serialSetInStore       bool
		serialChanged          bool
		hardwareUUIDSetInStore bool
		hardwareUUIDChanged    bool
		osquerySuccess         bool
		secretSetInStore       bool
		secretChanged          bool
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
			secretSetInStore:       true,
			secretChanged:          false,
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
			secretSetInStore:       true,
			secretChanged:          false,
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
			secretSetInStore:       true,
			secretChanged:          false,
			secretLivesInFile:      true,
			secretReadable:         true,
			expectDatabaseWipe:     true,
		},
		{
			name:                   "enroll secret changed, database wipe",
			serialSetInStore:       true,
			serialChanged:          false,
			hardwareUUIDSetInStore: true,
			hardwareUUIDChanged:    false,
			osquerySuccess:         true,
			secretSetInStore:       true,
			secretChanged:          true,
			secretLivesInFile:      true,
			secretReadable:         true,
			expectDatabaseWipe:     true,
		},
		{
			name:                   "enroll secret changed, enroll secret does not live in file, database wipe",
			serialSetInStore:       true,
			serialChanged:          false,
			hardwareUUIDSetInStore: true,
			hardwareUUIDChanged:    false,
			osquerySuccess:         true,
			secretSetInStore:       true,
			secretChanged:          true,
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
			secretSetInStore:       true,
			secretChanged:          true,
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
			secretSetInStore:       true,
			secretChanged:          false,
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
			secretSetInStore:       true,
			secretChanged:          false,
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
			secretSetInStore:       true,
			secretChanged:          false,
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
			secretSetInStore:       true,
			secretChanged:          false,
			secretLivesInFile:      true,
			secretReadable:         true,
			expectDatabaseWipe:     false,
		},
		{
			name:                   "secret not previously stored, other data unchanged, no database wipe",
			serialSetInStore:       true,
			serialChanged:          false,
			hardwareUUIDSetInStore: true,
			hardwareUUIDChanged:    false,
			osquerySuccess:         true,
			secretSetInStore:       false,
			secretChanged:          false,
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
			secretSetInStore:       true,
			secretChanged:          false,
			secretLivesInFile:      true,
			secretReadable:         true,
			expectDatabaseWipe:     true,
		},
	}

	for _, tt := range testCases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Set up dependencies: data store for hardware-identifying data
			testStore, err := storageci.NewStore(t, log.NewNopLogger(), storage.HostDataStore.String())
			require.NoError(t, err, "could not create test store")
			mockKnapsack := typesmocks.NewKnapsack(t)
			mockKnapsack.On("HostDataStore").Return(testStore)
			mockKnapsack.On("Stores").Return(map[storage.Store]types.KVStore{storage.HostDataStore: testStore}).Maybe()
			testBboltDB := storageci.SetupDB(t)
			mockKnapsack.On("BboltDB").Return(testBboltDB).Maybe()

			// Set up logger
			slogger := multislogger.New().Logger
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
					require.NoError(t, testStore.Set(hostDataKeySerial, []byte("some-old-serial")), "could not set serial in test store")
				} else {
					require.NoError(t, testStore.Set(hostDataKeySerial, []byte(actualSerial)), "could not set serial in test store")
				}
			}

			if tt.hardwareUUIDSetInStore {
				if tt.hardwareUUIDChanged {
					require.NoError(t, testStore.Set(hostDataKeyHardwareUuid, []byte("some-old-hardware-uuid")), "could not set hardware uuid in test store")
				} else {
					require.NoError(t, testStore.Set(hostDataKeyHardwareUuid, []byte(actualHardwareUUID)), "could not set hardware uuid in test store")
				}
			}

			// Set up dependencies: ensure that retrieved secret matches current data
			secretValue := []byte("abcd")

			if tt.secretLivesInFile {
				var secretFilepath string
				if tt.secretReadable {
					secretDir := t.TempDir()
					secretFilepath = filepath.Join(secretDir, "test-secret")
					require.NoError(t, os.WriteFile(secretFilepath, secretValue, 0644), "could not write out test secret")
				} else {
					secretFilepath = filepath.Join("not", "a", "real", "enroll", "secret")
				}

				mockKnapsack.On("EnrollSecret").Return("")
				mockKnapsack.On("EnrollSecretPath").Return(secretFilepath)
			} else {
				mockKnapsack.On("EnrollSecret").Return(string(secretValue))
			}

			if tt.secretSetInStore {
				if tt.secretChanged {
					require.NoError(t, testStore.Set(hostDataKeySecret, []byte("some-old-secret")), "could not set secret in test store")
				} else {
					require.NoError(t, testStore.Set(hostDataKeySecret, secretValue), "could not set secret in test store")
				}
			}

			// Set up dependencies: make a root directory for the backup to be stored in
			rootDir := t.TempDir()
			if tt.expectDatabaseWipe {
				mockKnapsack.On("RootDirectory").Return(rootDir)
			}

			// Set up dependencies: set an extra key that we know should be deleted when the store
			// is wiped
			extraKey := []byte("test_key")
			extraValue := []byte("this is a test value")
			require.NoError(t, testStore.Set(extraKey, extraValue), "could not set value in test store")

			// Make test call
			ResetDatabaseIfNeeded(context.TODO(), mockKnapsack)

			// Confirm backup occurred, if database got wiped
			if tt.expectDatabaseWipe {
				// Confirm the zip file exists
				expectedBackupLocation := filepath.Join(rootDir, "launcher.db.bak.zip")
				_, err = os.Stat(expectedBackupLocation)
				require.NoError(t, err, "expected file to exist at location:", expectedBackupLocation)

				// Confirm the zip is valid/readable
				zipReader, err := zip.OpenReader(expectedBackupLocation)
				require.NoError(t, err, "could not open zip reader")
				defer zipReader.Close()

				// Confirm the zip contains the backup file
				expectedBackupFile := "launcher.db.bak"
				backupFound := false
				for _, f := range zipReader.File {
					if f.Name != expectedBackupFile {
						continue
					}

					backupFound = true
				}

				require.True(t, backupFound, "backup not found in zip")
			}

			// Confirm whether the database got wiped
			v, err := testStore.Get(extraKey)
			require.NoError(t, err)
			if tt.expectDatabaseWipe {
				require.Nil(t, v, "database not wiped")
			} else {
				require.Equal(t, extraValue, v, "database wiped")
			}

			// Confirm hardware-identifying data got set regardless of db wipe,
			// as long as it was available
			if tt.osquerySuccess {
				serial, err := testStore.Get(hostDataKeySerial)
				require.NoError(t, err, "could not get serial from test store")
				require.Equal(t, actualSerial, string(serial), "serial in test store does not match expected serial")
				hardwareUUID, err := testStore.Get(hostDataKeyHardwareUuid)
				require.NoError(t, err, "could not get hardware UUID from test store")
				require.Equal(t, actualHardwareUUID, string(hardwareUUID), "hardware UUID in test store does not match expected hardware UUID")
			}
			if tt.secretReadable {
				secret, err := testStore.Get(hostDataKeySecret)
				require.NoError(t, err, "could not get secret from test store")
				require.Equal(t, secretValue, secret, "secret in test store does not match expected secret")
			}

			// Make sure all the functions were called as expected
			mockKnapsack.AssertExpectations(t)
		})
	}
}
