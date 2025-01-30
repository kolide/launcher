package storageci

import (
	"context"
	"fmt"
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
	var dbDir string
	if t != nil {
		dbDir = t.TempDir()
	} else {
		var err error
		dbDir, err = os.MkdirTemp(os.TempDir(), "storage-bbolt")
		if err != nil {
			fmt.Println("Failed to create temp dir for bbolt test")
			os.Exit(1) //nolint:forbidigo // Fine to use os.Exit inside tests
		}
	}

	// Create database; ensure we clean it up after the test
	db, err := bbolt.Open(filepath.Join(dbDir, dbTestFileName), 0600, nil)

	if t != nil {
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, db.Close())
		})
	} else {
		if err != nil {
			fmt.Println("Falied to create bolt db")
			os.Exit(1) //nolint:forbidigo // Fine to use os.Exit inside tests
		}
	}

	return db
}
