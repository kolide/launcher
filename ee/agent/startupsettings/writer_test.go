package startupsettings

import (
	"context"
	"testing"

	_ "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	agentsqlite "github.com/kolide/launcher/ee/agent/storage/sqlite"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestOpenWriter_NewDatabase(t *testing.T) {
	t.Parallel()

	// Set up dependencies
	testRootDir := t.TempDir()
	k := typesmocks.NewKnapsack(t)
	k.On("RootDirectory").Return(testRootDir)
	k.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel)
	updateChannelVal := "stable"
	k.On("UpdateChannel").Return(updateChannelVal)

	// Set up storage db, which should create the database and set all flags
	s, err := OpenWriter(context.TODO(), k)
	require.NoError(t, err, "expected no error setting up storage db")

	// Check that all values were set
	v1, err := s.kvStore.Get([]byte(keys.UpdateChannel.String()))
	require.NoError(t, err, "getting startup value")
	require.Equal(t, updateChannelVal, string(v1), "incorrect flag value")

	require.NoError(t, s.Close(), "closing startup db")
}

func TestOpenWriter_DatabaseAlreadyExists(t *testing.T) {
	t.Parallel()

	// Set up preexisting database
	testRootDir := setupTestDb(t)
	store, err := agentsqlite.OpenRW(context.TODO(), testRootDir, agentsqlite.StartupSettingsStore)
	require.NoError(t, err, "getting connection to test db")
	require.NoError(t, store.Set([]byte(keys.UpdateChannel.String()), []byte("some_old_value")), "setting key")

	// Confirm flags were set
	v1, err := store.Get([]byte(keys.UpdateChannel.String()))
	require.NoError(t, err, "getting startup value")
	require.Equal(t, "some_old_value", string(v1), "incorrect flag value")

	require.NoError(t, store.Close(), "closing setup connection")

	// Set up dependencies
	k := typesmocks.NewKnapsack(t)
	k.On("RootDirectory").Return(testRootDir)
	k.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel)

	// Set up flag
	updateChannelVal := "alpha"
	k.On("UpdateChannel").Return(updateChannelVal)

	// Set up storage db, which should create the database and set all flags
	s, err := OpenWriter(context.TODO(), k)
	require.NoError(t, err, "expected no error setting up storage db")

	// Now check that all values were updated
	v1, err = s.kvStore.Get([]byte(keys.UpdateChannel.String()))
	require.NoError(t, err, "getting startup value")
	require.Equal(t, updateChannelVal, string(v1), "incorrect flag value")

	require.NoError(t, s.Close(), "closing startup db")
}

func TestFlagsChanged(t *testing.T) {
	t.Parallel()

	// Set up dependencies
	testRootDir := t.TempDir()
	k := typesmocks.NewKnapsack(t)
	k.On("RootDirectory").Return(testRootDir)
	k.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel)
	updateChannelVal := "beta"
	k.On("UpdateChannel").Return(updateChannelVal).Once()

	// Set up storage db, which should create the database and set all flags
	s, err := OpenWriter(context.TODO(), k)
	require.NoError(t, err, "expected no error setting up storage db")

	// Check that all values were set
	v1, err := s.kvStore.Get([]byte(keys.UpdateChannel.String()))
	require.NoError(t, err, "getting startup value")
	require.Equal(t, updateChannelVal, string(v1), "incorrect flag value")

	// Now, prepare for flag changes
	newFlagValue := "alpha"
	k.On("UpdateChannel").Return(newFlagValue).Once()

	// Call FlagsChanged and expect that all flag values are updated
	s.FlagsChanged(keys.UpdateChannel)
	v1, err = s.kvStore.Get([]byte(keys.UpdateChannel.String()))
	require.NoError(t, err, "getting startup value")
	require.Equal(t, newFlagValue, string(v1), "incorrect flag value")

	require.NoError(t, s.Close(), "closing startup db")
}

func setupTestDb(t *testing.T) string {
	tempRootDir := t.TempDir()

	store, err := agentsqlite.OpenRW(context.TODO(), tempRootDir, agentsqlite.StartupSettingsStore)
	require.NoError(t, err, "setting up db connection")
	require.NoError(t, store.Close(), "closing test db")

	return tempRootDir
}
