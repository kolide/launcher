package startup

import (
	"context"
	"testing"

	_ "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/kolide/launcher/ee/agent/flags"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	agentsqlite "github.com/kolide/launcher/ee/agent/storage/sqlite"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestGetStartupValue(t *testing.T) {
	t.Parallel()

	testRootDir := setupTestDb(t)

	// Set flag value
	flagKey := keys.UpdateChannel.String()
	flagVal := "test value"
	store, err := agentsqlite.NewStore(context.TODO(), testRootDir, agentsqlite.TableKeyValuePairs)
	require.NoError(t, err, "getting connection to test db")
	require.NoError(t, store.Set([]byte(flagKey), []byte(flagVal)), "setting key")
	require.NoError(t, store.Close(), "closing setup connection")

	returnedVal, err := GetStartupValue(context.TODO(), testRootDir, flagKey)
	require.NoError(t, err, "expected no error getting startup value")
	require.Equal(t, flagVal, returnedVal, "flag value mismatch")
}

func TestGetStartupValue_DbNotExist(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()
	flagKey := keys.UpdateChannel.String()

	_, err := GetStartupValue(context.TODO(), testRootDir, flagKey)
	require.Error(t, err, "expected error getting startup value when database does not exist")
}

func TestNewStartupDatabase_NewDatabase(t *testing.T) {
	t.Parallel()

	// Set up dependencies
	testRootDir := t.TempDir()
	k := typesmocks.NewKnapsack(t)
	k.On("RootDirectory").Return(testRootDir)
	k.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel)
	k.On("RegisterChangeObserver", mock.Anything, keys.UseTUFAutoupdater)
	updateChannelVal := "stable"
	k.On("UpdateChannel").Return(updateChannelVal)
	k.On("UseTUFAutoupdater").Return(false)

	// Set up storage db, which should create the database and set all flags
	s, err := NewStartupDatabase(context.TODO(), k)
	require.NoError(t, err, "expected no error setting up storage db")

	require.NoError(t, s.Close(), "closing startup db")

	// Check that all values were set
	v1, err := GetStartupValue(context.TODO(), testRootDir, keys.UpdateChannel.String())
	require.NoError(t, err, "getting startup value")
	require.Equal(t, updateChannelVal, v1, "incorrect flag value")
	v2, err := GetStartupValue(context.TODO(), testRootDir, keys.UseTUFAutoupdater.String())
	require.NoError(t, err, "getting startup value")
	require.False(t, flags.StringToBool(v2), "incorrect flag value")
}

func TestNewStartupDatabase_DatabaseAlreadyExists(t *testing.T) {
	t.Parallel()

	// Set up preexisting database
	testRootDir := setupTestDb(t)
	store, err := agentsqlite.NewStore(context.TODO(), testRootDir, agentsqlite.TableKeyValuePairs)
	require.NoError(t, err, "getting connection to test db")
	require.NoError(t, store.Set([]byte(keys.UpdateChannel.String()), []byte("some_old_value")), "setting key")
	require.NoError(t, store.Set([]byte(keys.UseTUFAutoupdater.String()), []byte(flags.BoolToString(false))), "setting key")
	require.NoError(t, store.Close(), "closing setup connection")

	// Confirm flags were set
	v1, err := GetStartupValue(context.TODO(), testRootDir, keys.UpdateChannel.String())
	require.NoError(t, err, "getting startup value")
	require.Equal(t, "some_old_value", v1, "incorrect flag value")
	v2, err := GetStartupValue(context.TODO(), testRootDir, keys.UseTUFAutoupdater.String())
	require.NoError(t, err, "getting startup value")
	require.False(t, flags.StringToBool(v2), "incorrect flag value")

	// Set up dependencies
	k := typesmocks.NewKnapsack(t)
	k.On("RootDirectory").Return(testRootDir)
	k.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel)
	k.On("RegisterChangeObserver", mock.Anything, keys.UseTUFAutoupdater)

	// Set up flag
	updateChannelVal := "alpha"
	k.On("UpdateChannel").Return(updateChannelVal)
	k.On("UseTUFAutoupdater").Return(true)

	// Set up storage db, which should create the database and set all flags
	s, err := NewStartupDatabase(context.TODO(), k)
	require.NoError(t, err, "expected no error setting up storage db")

	// Close the db to flush changes
	require.NoError(t, s.Close(), "closing startup db")

	// Now check that all values were updated
	v1, err = GetStartupValue(context.TODO(), testRootDir, keys.UpdateChannel.String())
	require.NoError(t, err, "getting startup value")
	require.Equal(t, updateChannelVal, v1, "incorrect flag value")
	v2, err = GetStartupValue(context.TODO(), testRootDir, keys.UseTUFAutoupdater.String())
	require.NoError(t, err, "getting startup value")
	require.True(t, flags.StringToBool(v2), "incorrect flag value")
}

func TestFlagsChanged(t *testing.T) {
	t.Parallel()

	// Set up dependencies
	testRootDir := t.TempDir()
	k := typesmocks.NewKnapsack(t)
	k.On("RootDirectory").Return(testRootDir)
	k.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel)
	k.On("RegisterChangeObserver", mock.Anything, keys.UseTUFAutoupdater)
	updateChannelVal := "beta"
	k.On("UpdateChannel").Return(updateChannelVal).Once()
	useTufAutoupdaterVal := true
	k.On("UseTUFAutoupdater").Return(useTufAutoupdaterVal).Once()

	// Set up storage db, which should create the database and set all flags
	s, err := NewStartupDatabase(context.TODO(), k)
	require.NoError(t, err, "expected no error setting up storage db")

	// Check that all values were set
	v1, err := GetStartupValue(context.TODO(), testRootDir, keys.UpdateChannel.String())
	require.NoError(t, err, "getting startup value")
	require.Equal(t, updateChannelVal, v1, "incorrect flag value")
	v2, err := GetStartupValue(context.TODO(), testRootDir, keys.UseTUFAutoupdater.String())
	require.NoError(t, err, "getting startup value")
	require.Equal(t, useTufAutoupdaterVal, flags.StringToBool(v2), "incorrect flag value")

	// Now, prepare for flag changes
	newFlagValue := "alpha"
	k.On("UpdateChannel").Return(newFlagValue).Once()
	newUseTufAutoupdaterVal := false
	k.On("UseTUFAutoupdater").Return(newUseTufAutoupdaterVal).Once()

	// Call FlagsChanged and expect that all flag values are updated
	s.FlagsChanged(keys.UpdateChannel)
	v1, err = GetStartupValue(context.TODO(), testRootDir, keys.UpdateChannel.String())
	require.NoError(t, err, "getting startup value")
	require.Equal(t, newFlagValue, v1, "incorrect flag value")
	v2, err = GetStartupValue(context.TODO(), testRootDir, keys.UseTUFAutoupdater.String())
	require.NoError(t, err, "getting startup value")
	require.Equal(t, newUseTufAutoupdaterVal, flags.StringToBool(v2), "incorrect flag value")

	require.NoError(t, s.Close(), "closing startup db")
}

func setupTestDb(t *testing.T) string {
	tempRootDir := t.TempDir()

	store, err := agentsqlite.NewStore(context.TODO(), tempRootDir, agentsqlite.TableKeyValuePairs)
	require.NoError(t, err, "setting up db connection")
	require.NoError(t, store.Close(), "closing test db")

	return tempRootDir
}
