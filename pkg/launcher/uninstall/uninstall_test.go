package uninstall

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
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

			testDir := t.TempDir()
			dbPath := filepath.Join(testDir, "db")
			enrollSecretPath := filepath.Join(testDir, "enroll_secret")

			db, err := bbolt.Open(dbPath, 0600, nil)
			require.NoError(t, err)

			// create file in test dir
			_, err = os.Create(enrollSecretPath)
			require.NoError(t, err)

			// sanity check that the files exist
			_, err = os.Stat(enrollSecretPath)
			require.NoError(t, err)
			_, err = os.Stat(dbPath)
			require.NoError(t, err)

			k := mocks.NewKnapsack(t)
			k.On("BboltDB").Return(db)
			k.On("EnrollSecretPath").Return(enrollSecretPath)
			k.On("Slogger").Return(multislogger.New().Logger)

			uninstall(context.TODO(), k)

			// check that file was deleted
			_, err = os.Stat(enrollSecretPath)
			require.True(t, os.IsNotExist(err))

			// check that db was deleted
			_, err = os.Stat(dbPath)
			require.True(t, os.IsNotExist(err))
		})
	}
}
