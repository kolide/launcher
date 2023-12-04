package agent

import (
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
			require.NoError(t, err)
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
					require.NoError(t, testStore.Set(hostDataKeySerial, []byte("some-old-serial")))
				} else {
					require.NoError(t, testStore.Set(hostDataKeySerial, []byte(actualSerial)))
				}
			}

			if tt.hardwareUUIDSetInStore {
				if tt.hardwareUUIDChanged {
					require.NoError(t, testStore.Set(hostDataKeyHardwareUuid, []byte("some-old-hardware-uuid")))
				} else {
					require.NoError(t, testStore.Set(hostDataKeyHardwareUuid, []byte(actualHardwareUUID)))
				}
			}

			// Set up dependencies: ensure that retrieved secret matches current data
			secretValue := []byte("abcd")

			var secretFilepath string
			if tt.secretReadable {
				secretDir := t.TempDir()
				secretFilepath = filepath.Join(secretDir, "test-secret")
				require.NoError(t, os.WriteFile(secretFilepath, secretValue, 0644))
			} else {
				secretFilepath = filepath.Join("not", "a", "real", "enroll", "secret")
			}

			mockKnapsack.On("EnrollSecret").Return("")
			mockKnapsack.On("EnrollSecretPath").Return(secretFilepath)

			if tt.secretSetInStore {
				if tt.secretChanged {
					require.NoError(t, testStore.Set(hostDataKeySecret, []byte("some-old-secret")))
				} else {
					require.NoError(t, testStore.Set(hostDataKeySecret, secretValue))
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
			require.NoError(t, testStore.Set(extraKey, extraValue))

			// Make test call
			ResetDatabaseIfNeeded(context.TODO(), mockKnapsack)

			// Confirm backup occurred, if database got wiped
			if tt.expectDatabaseWipe {
				expectedBackupLocation := filepath.Join(rootDir, "launcher.db.bak.zip")
				_, err = os.Stat(expectedBackupLocation)
				require.NoError(t, err)
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
				require.NoError(t, err)
				require.Equal(t, actualSerial, string(serial))
				hardwareUUID, err := testStore.Get(hostDataKeyHardwareUuid)
				require.NoError(t, err)
				require.Equal(t, actualHardwareUUID, string(hardwareUUID))
			}
			if tt.secretReadable {
				secret, err := testStore.Get(hostDataKeySecret)
				require.NoError(t, err)
				require.Equal(t, secretValue, secret)
			}

			// Make sure all the functions were called as expected
			mockKnapsack.AssertExpectations(t)
		})
	}
}
