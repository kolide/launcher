package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

const (
	dbTestFileName = "test.db"
)

func NewCIKeyValueStore(t *testing.T, logger log.Logger, bucketName string) types.GetterSetterDeleterIterator {
	if os.Getenv("CI") == "true" {
		return NewInMemoryKeyValueStore(logger)
	}
	return NewBBoltKeyValueStore(logger, setupDB(t), bucketName)
}

func setupDB(t *testing.T) *bbolt.DB {
	// Create a temp directory to hold our bbolt db
	dbDir := t.TempDir()

	// Create database; ensure we clean it up after the test
	db, err := bbolt.Open(filepath.Join(dbDir, dbTestFileName), 0600, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	return db
}
