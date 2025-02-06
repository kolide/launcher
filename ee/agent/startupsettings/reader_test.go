package startupsettings

import (
	"context"
	"testing"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	agentsqlite "github.com/kolide/launcher/ee/agent/storage/sqlite"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func TestGet(t *testing.T) {
	t.Parallel()

	testRootDir := setupTestDb(t)

	// Set flag value
	flagKey := keys.UpdateChannel.String()
	flagVal := "test value"
	store, err := agentsqlite.OpenRW(context.TODO(), testRootDir, agentsqlite.StartupSettingsStore)
	require.NoError(t, err, "getting connection to test db")
	require.NoError(t, store.Set([]byte(flagKey), []byte(flagVal)), "setting key")
	require.NoError(t, store.Close(), "closing setup connection")

	r, err := OpenReader(context.TODO(), multislogger.NewNopLogger(), testRootDir)
	require.NoError(t, err, "creating reader")

	returnedVal, err := r.Get(flagKey)
	require.NoError(t, err, "expected no error getting startup value")
	require.Equal(t, flagVal, returnedVal, "flag value mismatch")

	require.NoError(t, r.Close(), err)
}

func TestGet_DbNotExist(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()
	flagKey := keys.UpdateChannel.String()

	r, err := OpenReader(context.TODO(), multislogger.NewNopLogger(), testRootDir)
	require.NoError(t, err, "creating reader")
	_, err = r.Get(flagKey)
	require.Error(t, err, "expected error getting startup value when database does not exist")

	require.NoError(t, r.Close(), err)
}
