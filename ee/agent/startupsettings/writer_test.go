package startupsettings

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	_ "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/storage/inmemory"
	agentsqlite "github.com/kolide/launcher/ee/agent/storage/sqlite"
	"github.com/kolide/launcher/ee/agent/types"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
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
	k.On("ConfigStore").Return(inmemory.NewStore())
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("KatcConfigStore").Return(inmemory.NewStore())
	k.On("RegistrationIDs").Return([]string{types.DefaultRegistrationID})

	// Set up storage db, which should create the database and set all flags
	s, err := OpenWriter(context.TODO(), k)
	require.NoError(t, err, "expected no error setting up storage db")

	require.NoError(t, s.WriteSettings(), "should be able to writing settings")

	// Check that all values were set
	v1, err := s.kvStore.Get([]byte(keys.UpdateChannel.String()))
	require.NoError(t, err, "getting startup value")
	require.Equal(t, updateChannelVal, string(v1), "incorrect flag value")

	v2, err := s.kvStore.Get([]byte("use_tuf_autoupdater"))
	require.NoError(t, err, "getting startup value")
	require.Equal(t, "enabled", string(v2), "incorrect flag value")

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
	k.On("KatcConfigStore").Return(inmemory.NewStore())
	k.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel)
	k.On("RegisterChangeObserver", mock.Anything, keys.PinnedLauncherVersion)
	k.On("RegisterChangeObserver", mock.Anything, keys.PinnedOsquerydVersion)
	k.On("RegistrationIDs").Return([]string{types.DefaultRegistrationID})

	// Set up flag
	updateChannelVal := "alpha"
	pinnedLauncherVersion := "1.6.0"
	pinnedOsquerydVersion := "5.11.0"
	k.On("UpdateChannel").Return(updateChannelVal)
	k.On("PinnedLauncherVersion").Return(pinnedLauncherVersion)
	k.On("PinnedOsquerydVersion").Return(pinnedOsquerydVersion)

	k.On("ConfigStore").Return(inmemory.NewStore())
	k.On("Slogger").Return(multislogger.NewNopLogger())

	// Set up storage db, which should create the database and set all flags
	s, err := OpenWriter(context.TODO(), k)
	require.NoError(t, err, "expected no error setting up storage db")

	require.NoError(t, s.WriteSettings(), "should be able to writing settings")

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
	k.On("KatcConfigStore").Return(inmemory.NewStore())
	k.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel)
	k.On("RegisterChangeObserver", mock.Anything, keys.PinnedLauncherVersion)
	k.On("RegisterChangeObserver", mock.Anything, keys.PinnedOsquerydVersion)
	k.On("RegistrationIDs").Return([]string{types.DefaultRegistrationID})
	updateChannelVal := "beta"
	k.On("UpdateChannel").Return(updateChannelVal).Once()
	pinnedLauncherVersion := "1.2.3"
	k.On("PinnedLauncherVersion").Return(pinnedLauncherVersion).Once()
	pinnedOsquerydVersion := "5.3.2"
	k.On("PinnedOsquerydVersion").Return(pinnedOsquerydVersion).Once()

	autoTableConstructionValue := ulid.New()

	configStore := inmemory.NewStore()
	configMap := map[string]any{
		"auto_table_construction":      autoTableConstructionValue,
		"something_else_not_important": ulid.New(),
	}
	configJson, err := json.Marshal(configMap)
	require.NoError(t, err, "marshalling config map")

	configStore.Set([]byte("config"), configJson)
	k.On("ConfigStore").Return(configStore)

	// Set up storage db, which should create the database and set all flags
	s, err := OpenWriter(context.TODO(), k)
	require.NoError(t, err, "expected no error setting up storage db")

	require.NoError(t, s.WriteSettings(), "should be able to writing settings")

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

	v4, err := s.kvStore.Get([]byte("auto_table_construction"))
	require.NoError(t, err, "getting startup value")
	require.Equal(t, fmt.Sprintf("{\"auto_table_construction\":\"%s\"}", autoTableConstructionValue), string(v4), "incorrect config value")

	// Now, prepare for flag changes
	newFlagValue := "alpha"
	k.On("UpdateChannel").Return(newFlagValue).Once()
	newPinnedLauncherVersion := ""
	k.On("PinnedLauncherVersion").Return(newPinnedLauncherVersion).Once()
	newPinnedOsquerydVersion := "5.4.3"
	k.On("PinnedOsquerydVersion").Return(newPinnedOsquerydVersion).Once()

	// Call FlagsChanged and expect that all flag values are updated
	s.FlagsChanged(context.TODO(), keys.UpdateChannel)
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
