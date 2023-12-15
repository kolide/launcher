package startup

import (
	"context"
	"os"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestGetStartupValue(t *testing.T) {
	t.Parallel()

	testRootDir := setupTestDb(t)

	// Set flag value
	conn, err := dbConn(context.TODO(), testRootDir)
	require.NoError(t, err, "getting connection to test db")
	flagKey := keys.UpdateChannel.String()
	flagVal := "test value"
	_, err = conn.Exec(`INSERT INTO startup_flag (flag_name, flag_value) VALUES (?, ?);`, flagKey, flagVal)
	require.NoError(t, err, "inserting flag value")
	require.NoError(t, conn.Close(), "closing db connection")

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
	agentFlagsStore, err := storageci.NewStore(t, log.NewNopLogger(), storage.AgentFlagsStore.String())
	require.NoError(t, err, "setting up agent flags store")
	k.On("AgentFlagsStore").Return(agentFlagsStore)
	k.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel)

	// Set up flags in agent flags store -- setting each value equal to the key name
	// for easy checking later
	require.NoError(t, agentFlagsStore.Set([]byte(keys.UpdateChannel.String()), []byte(keys.UpdateChannel.String())), "setting flag in store")

	// Set up storage db, which should create the database and set all flags
	_, err = NewStartupDatabase(context.TODO(), k)
	require.NoError(t, err, "expected no error setting up storage db")

	// Confirm the database exists
	_, err = os.Stat(dbLocation(testRootDir))
	require.NoError(t, err, "database not created")

	// Now check that all values were set
	v, err := GetStartupValue(context.TODO(), testRootDir, keys.UpdateChannel.String())
	require.NoError(t, err, "getting startup value")
	require.Equal(t, keys.UpdateChannel.String(), v, "incorrect flag value")
}

func TestNewStartupDatabase_DatabaseAlreadyExists(t *testing.T) {
	t.Parallel()

	// Set up preexisting database
	testRootDir := setupTestDb(t)
	_, err := os.Stat(dbLocation(testRootDir))
	require.NoError(t, err, "database not created")
	conn, err := dbConn(context.TODO(), testRootDir)
	require.NoError(t, err, "getting connection to test db")
	_, err = conn.Exec(`INSERT INTO startup_flag (flag_name, flag_value) VALUES (?, "some_old_value");`, keys.UpdateChannel.String())
	require.NoError(t, err, "setting old value in database")
	conn.Close()

	// Confirm flags were set
	v, err := GetStartupValue(context.TODO(), testRootDir, keys.UpdateChannel.String())
	require.NoError(t, err, "getting startup value")
	require.Equal(t, "some_old_value", v, "incorrect flag value")

	// Set up dependencies
	k := typesmocks.NewKnapsack(t)
	k.On("RootDirectory").Return(testRootDir)
	agentFlagsStore, err := storageci.NewStore(t, log.NewNopLogger(), storage.AgentFlagsStore.String())
	require.NoError(t, err, "setting up agent flags store")
	k.On("AgentFlagsStore").Return(agentFlagsStore)
	k.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel)

	// Set up flags in agent flags store -- setting each value equal to the key name
	// for easy checking later
	require.NoError(t, agentFlagsStore.Set([]byte(keys.UpdateChannel.String()), []byte(keys.UpdateChannel.String())), "setting flag in store")

	// Set up storage db, which should create the database and set all flags
	s, err := NewStartupDatabase(context.TODO(), k)
	require.NoError(t, err, "expected no error setting up storage db")

	// Close the db to flush changes
	require.NoError(t, s.Close(), "closing startup db")

	// Now check that all values were updated
	v, err = GetStartupValue(context.TODO(), testRootDir, keys.UpdateChannel.String())
	require.NoError(t, err, "getting startup value")
	require.Equal(t, keys.UpdateChannel.String(), v, "incorrect flag value")
}

func TestFlagsChanged(t *testing.T) {
	t.Parallel()

	// Set up dependencies
	testRootDir := t.TempDir()
	k := typesmocks.NewKnapsack(t)
	k.On("RootDirectory").Return(testRootDir)
	agentFlagsStore, err := storageci.NewStore(t, log.NewNopLogger(), storage.AgentFlagsStore.String())
	require.NoError(t, err, "setting up agent flags store")
	k.On("AgentFlagsStore").Return(agentFlagsStore)
	k.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel)

	// Set up flags in agent flags store -- setting each value equal to the key name
	// for easy checking later
	require.NoError(t, agentFlagsStore.Set([]byte(keys.UpdateChannel.String()), []byte(keys.UpdateChannel.String())), "setting flag in store")

	// Set up storage db, which should create the database and set all flags
	s, err := NewStartupDatabase(context.TODO(), k)
	require.NoError(t, err, "expected no error setting up storage db")

	// Confirm the database exists
	_, err = os.Stat(dbLocation(testRootDir))
	require.NoError(t, err, "database not created")

	// Now check that all values were set
	v, err := GetStartupValue(context.TODO(), testRootDir, keys.UpdateChannel.String())
	require.NoError(t, err, "getting startup value")
	require.Equal(t, keys.UpdateChannel.String(), v, "incorrect flag value")

	// Now, prepare for flag changes
	newFlagValue := "new_channel_val"
	require.NoError(t, agentFlagsStore.Set([]byte(keys.UpdateChannel.String()), []byte(newFlagValue)), "setting flag in store")

	// Call FlagsChanged and expect that all flag values are updated
	s.FlagsChanged(keys.UpdateChannel)
	v, err = GetStartupValue(context.TODO(), testRootDir, keys.UpdateChannel.String())
	require.NoError(t, err, "getting startup value")
	require.Equal(t, newFlagValue, v, "incorrect flag value")
}

func setupTestDb(t *testing.T) string {
	tempRootDir := t.TempDir()

	conn, err := dbConn(context.TODO(), tempRootDir)
	require.NoError(t, err, "setting up db connection")

	_, err = conn.Exec(`
	CREATE TABLE IF NOT EXISTS startup_flag (
		flag_id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
		flag_name TEXT NOT NULL UNIQUE,
		flag_value TEXT
	);`)
	require.NoError(t, err, "creating table")

	require.NoError(t, conn.Close(), "closing test db")

	return tempRootDir
}
