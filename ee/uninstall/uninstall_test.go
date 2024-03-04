package uninstall

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func TestUninstall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{
			name: "happy path",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// create a enroll secret to delete
			enrollSecretPath := filepath.Join(t.TempDir(), "enroll_secret")
			f, err := os.Create(enrollSecretPath)
			require.NoError(t, err)
			require.NoError(t, f.Close())

			// sanity check that the file exist
			_, err = os.Stat(enrollSecretPath)
			require.NoError(t, err)

			k := mocks.NewKnapsack(t)
			k.On("EnrollSecretPath").Return(enrollSecretPath)
			k.On("Slogger").Return(multislogger.NewNopLogger())
			testConfigStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.ConfigStore.String())
			require.NoError(t, err, "could not create test config store")
			k.On("ConfigStore").Return(testConfigStore)
			testHostDataStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.PersistentHostDataStore.String())
			require.NoError(t, err, "could not create test host data store")
			k.On("PersistentHostDataStore").Return(testHostDataStore)
			testServerProvidedDataStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.ServerProvidedDataStore.String())
			require.NoError(t, err, "could not create test server provided data store")
			k.On("ServerProvidedDataStore").Return(testServerProvidedDataStore)
			stores := map[storage.Store]types.KVStore{
				storage.PersistentHostDataStore: testHostDataStore,
				storage.ConfigStore:             testConfigStore,
				storage.ServerProvidedDataStore: testServerProvidedDataStore,
			}
			k.On("Stores").Return(stores)
			testSerial := []byte("C999999999")
			testHardwareUUID := []byte("99999999-9999-9999-9999-999999999999")

			// seed the test storage with known serial and hardware_uuids to test against the reset records later
			require.NoError(t, testHostDataStore.Set([]byte("serial"), testSerial), "could not set serial in test store")
			require.NoError(t, testHostDataStore.Set([]byte("hardware_uuid"), testHardwareUUID), "could not set hardware uuid in test store")
			// additionally seed all stores with some data to ensure we are clearing all values later
			for _, store := range stores {
				for j := 0; j < 3; j++ {
					require.NoError(t, store.Set([]byte(fmt.Sprint(j)), []byte(fmt.Sprint(j))))
				}

				require.NoError(t, err)
			}

			Uninstall(context.TODO(), k, false)

			// check that file was deleted
			_, err = os.Stat(enrollSecretPath)
			require.True(t, os.IsNotExist(err))

			// check that all stores are empty except for the uninstallation history
			itemsFound := 0
			for _, store := range stores {
				store.ForEach(
					func(k, v []byte) error {
						itemsFound++
						return nil
					},
				)
			}

			// the expectation of 1 here is coming from the single remaining reset_records key
			// see agent.ResetDatabase for additional context
			require.Equal(t, 1, itemsFound)
			resetRecordsRaw, err := testHostDataStore.Get(agent.HostDataKeyResetRecords)
			require.NoError(t, err, "could not get reset records from test store")
			var resetRecords []agent.DBResetRecord
			require.Greater(t, len(resetRecordsRaw), 0, "did not expect reset records to be empty")
			err = json.Unmarshal(resetRecordsRaw, &resetRecords)
			require.NoError(t, err, "expected to be able to unmarshal reset records")
			require.Equal(t, 1, len(resetRecords), "expected reset records to contain exactly 1 uninstallation record")
			// now check the individual bits we want to ensure are migrated to the reset record
			resetRecord := resetRecords[0]
			require.Equal(t, resetReasonUninstallRequested, resetRecord.ResetReason, "expected reset record to indicate the uninstall requested")
			require.Equal(t, string(testSerial), resetRecord.Serial, "expected reset record to indicate the serial number from the original installation")
			require.Equal(t, string(testHardwareUUID), resetRecord.HardwareUUID, "expected reset record to indicate the hardware UUID from the original installation")
		})
	}
}
