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
	k.On("RegisterChangeObserver", mock.Anything, keys.PinnedLauncherVersion)
	k.On("RegisterChangeObserver", mock.Anything, keys.PinnedOsquerydVersion)
	updateChannelVal := "stable"
	k.On("UpdateChannel").Return(updateChannelVal)
	k.On("PinnedLauncherVersion").Return("")
	k.On("PinnedOsquerydVersion").Return("5.11.0")

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
	require.NoError(t, store.Set([]byte(keys.PinnedLauncherVersion.String()), []byte("")), "setting key")
	require.NoError(t, store.Set([]byte(keys.PinnedOsquerydVersion.String()), []byte("")), "setting key")

	// Confirm flags were set
	v1, err := store.Get([]byte(keys.UpdateChannel.String()))
	require.NoError(t, err, "getting startup value")
	require.Equal(t, "some_old_value", string(v1), "incorrect flag value")

	v2, err := store.Get([]byte(keys.PinnedLauncherVersion.String()))
	require.NoError(t, err, "getting startup value")
	require.Equal(t, "", string(v2), "incorrect flag value")

	v3, err := store.Get([]byte(keys.PinnedOsquerydVersion.String()))
	require.NoError(t, err, "getting startup value")
	require.Equal(t, "", string(v3), "incorrect flag value")

	require.NoError(t, store.Close(), "closing setup connection")

	// Set up dependencies
	k := typesmocks.NewKnapsack(t)
	k.On("RootDirectory").Return(testRootDir)
	k.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel)
	k.On("RegisterChangeObserver", mock.Anything, keys.PinnedLauncherVersion)
	k.On("RegisterChangeObserver", mock.Anything, keys.PinnedOsquerydVersion)

	// Set up flag
	updateChannelVal := "alpha"
	pinnedLauncherVersion := "1.6.0"
	pinnedOsquerydVersion := "5.11.0"
	k.On("UpdateChannel").Return(updateChannelVal)
	k.On("PinnedLauncherVersion").Return(pinnedLauncherVersion)
	k.On("PinnedOsquerydVersion").Return(pinnedOsquerydVersion)

	// Set up storage db, which should create the database and set all flags
	s, err := OpenWriter(context.TODO(), k)
	require.NoError(t, err, "expected no error setting up storage db")

	// Now check that all values were updated
	v1, err = s.kvStore.Get([]byte(keys.UpdateChannel.String()))
	require.NoError(t, err, "getting startup value")
	require.Equal(t, updateChannelVal, string(v1), "incorrect flag value")

	v2, err = s.kvStore.Get([]byte(keys.PinnedLauncherVersion.String()))
	require.NoError(t, err, "getting startup value")
	require.Equal(t, pinnedLauncherVersion, string(v2), "incorrect flag value")

	v3, err = s.kvStore.Get([]byte(keys.PinnedOsquerydVersion.String()))
	require.NoError(t, err, "getting startup value")
	require.Equal(t, pinnedOsquerydVersion, string(v3), "incorrect flag value")

	require.NoError(t, s.Close(), "closing startup db")
}

func TestFlagsChanged(t *testing.T) {
	t.Parallel()

	// Set up dependencies
	testRootDir := t.TempDir()
	k := typesmocks.NewKnapsack(t)
	k.On("RootDirectory").Return(testRootDir)
	k.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel)
	k.On("RegisterChangeObserver", mock.Anything, keys.PinnedLauncherVersion)
	k.On("RegisterChangeObserver", mock.Anything, keys.PinnedOsquerydVersion)
	updateChannelVal := "beta"
	k.On("UpdateChannel").Return(updateChannelVal).Once()
	pinnedLauncherVersion := "1.2.3"
	k.On("PinnedLauncherVersion").Return(pinnedLauncherVersion).Once()
	pinnedOsquerydVersion := "5.3.2"
	k.On("PinnedOsquerydVersion").Return(pinnedOsquerydVersion).Once()

	// Set up storage db, which should create the database and set all flags
	s, err := OpenWriter(context.TODO(), k)
	require.NoError(t, err, "expected no error setting up storage db")

	// Check that all values were set
	v1, err := s.kvStore.Get([]byte(keys.UpdateChannel.String()))
	require.NoError(t, err, "getting startup value")
	require.Equal(t, updateChannelVal, string(v1), "incorrect flag value")

	v2, err := s.kvStore.Get([]byte(keys.PinnedLauncherVersion.String()))
	require.NoError(t, err, "getting startup value")
	require.Equal(t, pinnedLauncherVersion, string(v2), "incorrect flag value")

	v3, err := s.kvStore.Get([]byte(keys.PinnedOsquerydVersion.String()))
	require.NoError(t, err, "getting startup value")
	require.Equal(t, pinnedOsquerydVersion, string(v3), "incorrect flag value")

	// Now, prepare for flag changes
	newFlagValue := "alpha"
	k.On("UpdateChannel").Return(newFlagValue).Once()
	newPinnedLauncherVersion := ""
	k.On("PinnedLauncherVersion").Return(newPinnedLauncherVersion).Once()
	newPinnedOsquerydVersion := "5.4.3"
	k.On("PinnedOsquerydVersion").Return(newPinnedOsquerydVersion).Once()

	// Call FlagsChanged and expect that all flag values are updated
	s.FlagsChanged(keys.UpdateChannel)
	v1, err = s.kvStore.Get([]byte(keys.UpdateChannel.String()))
	require.NoError(t, err, "getting startup value")
	require.Equal(t, newFlagValue, string(v1), "incorrect flag value")

	v2, err = s.kvStore.Get([]byte(keys.PinnedLauncherVersion.String()))
	require.NoError(t, err, "getting startup value")
	require.Equal(t, newPinnedLauncherVersion, string(v2), "incorrect flag value")

	v3, err = s.kvStore.Get([]byte(keys.PinnedOsquerydVersion.String()))
	require.NoError(t, err, "getting startup value")
	require.Equal(t, newPinnedOsquerydVersion, string(v3), "incorrect flag value")

	require.NoError(t, s.Close(), "closing startup db")
}

func setupTestDb(t *testing.T) string {
	tempRootDir := t.TempDir()

	store, err := agentsqlite.OpenRW(context.TODO(), tempRootDir, agentsqlite.StartupSettingsStore)
	require.NoError(t, err, "setting up db connection")
	require.NoError(t, store.Close(), "closing test db")

	return tempRootDir
}
