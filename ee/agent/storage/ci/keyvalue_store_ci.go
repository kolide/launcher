package storageci

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	agentbbolt "github.com/kolide/launcher/ee/agent/storage/bbolt"
	"github.com/kolide/launcher/ee/agent/storage/inmemory"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

const (
	dbTestFileName = "test.db"
)

func NewStore(t *testing.T, slogger *slog.Logger, bucketName string) (types.KVStore, error) {
	if os.Getenv("CI") == "true" {
		return inmemory.NewStore(), nil
	}

	return agentbbolt.NewStore(context.TODO(), slogger, SetupDB(t), bucketName)
}

// SetupDB is used for creating bbolt databases for testing
func SetupDB(t *testing.T) *bbolt.DB {
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
