package keys

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

func TestSetupLocalDbKey(t *testing.T) {
	t.Parallel()

	db := setupDb(t)
	logger := log.NewJSONLogger(os.Stderr) //log.NewNopLogger()

	key, err := SetupLocalDbKey(logger, db)
	require.NoError(t, err)
	require.NotNil(t, key)

	// Call a thing. Make sure this is a real key
	require.NotNil(t, key.Public())

	// If we call this _again_ do we get the same key back?
	key2, err := SetupLocalDbKey(logger, db)
	require.NoError(t, err)
	require.Equal(t, key.Public(), key2.Public())

}

func setupDb(t *testing.T) *bbolt.DB {
	// Create a temp directory to hold our bbolt db
	dbDir := t.TempDir()

	// Create database; ensure we clean it up after the test
	db, err := bbolt.Open(filepath.Join(dbDir, "test.db"), 0600, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	return db
}
