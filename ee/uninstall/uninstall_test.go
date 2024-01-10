package uninstall

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/kit/ulid"
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

			// create 3 stores with 3 items each
			stores := map[storage.Store]types.KVStore{}
			for i := 0; i < 3; i++ {
				store, err := storageci.NewStore(t, log.NewNopLogger(), ulid.New())
				require.NoError(t, err)

				for j := 0; j < 3; j++ {
					require.NoError(t, store.Set([]byte(fmt.Sprint(j)), []byte(fmt.Sprint(j))))
				}

				require.NoError(t, err)
				stores[storage.Store(fmt.Sprint(i))] = store
			}

			// sanity check that we have 3 stores with 3 items each
			itemsExpected := 9
			itemsFound := 0
			for _, store := range stores {
				store.ForEach(
					func(k, v []byte) error {
						itemsFound++
						return nil
					},
				)
			}
			require.Equal(t, itemsExpected, itemsFound)

			k := mocks.NewKnapsack(t)
			k.On("Stores").Return(stores)
			k.On("EnrollSecretPath").Return(enrollSecretPath)
			k.On("Slogger").Return(multislogger.New().Logger)

			uninstallNoExit(context.TODO(), k)

			// check that file was deleted
			_, err = os.Stat(enrollSecretPath)
			require.True(t, os.IsNotExist(err))

			// check that all stores are empty
			itemsFound = 0
			for _, store := range stores {
				store.ForEach(
					func(k, v []byte) error {
						itemsFound++
						return nil
					},
				)
			}
			require.Equal(t, 0, itemsFound)
		})
	}
}
