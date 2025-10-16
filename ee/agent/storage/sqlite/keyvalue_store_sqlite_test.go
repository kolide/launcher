package agentsqlite

import (
	"fmt"
	"os"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func TestOpenRO_DatabaseExists(t *testing.T) {
	t.Parallel()

	// Create database
	testRootDir := t.TempDir()
	s1, err := OpenRW(t.Context(), testRootDir, StartupSettingsStore)
	require.NoError(t, err, "setting up database")
	require.NoError(t, s1.Close(), "closing database")

	// Create RO-connection to database
	s2, err := OpenRO(t.Context(), multislogger.NewNopLogger(), testRootDir, StartupSettingsStore)
	require.NoError(t, err, "setting up database")
	require.NoError(t, s2.Close(), "closing database")
}

func TestOpenRO_DatabaseDoesNotExist(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()

	s, err := OpenRO(t.Context(), multislogger.NewNopLogger(), testRootDir, StartupSettingsStore)
	require.NoError(t, err, "no validation should be performed on RO connection")
	require.NoFileExists(t, dbLocation(testRootDir), "database should not have been created")
	require.NoError(t, s.Close(), "closing database")
}

func TestOpenRW_EmptyFileExists(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()
	dbFile := dbLocation(testRootDir)

	// Create empty file
	f, err := os.OpenFile(dbFile, os.O_RDONLY|os.O_CREATE, 0666)
	require.NoError(t, err, "creating empty file")
	require.NoError(t, f.Close(), "closing empty db file")

	s, err := OpenRW(t.Context(), testRootDir, StartupSettingsStore)
	require.NoError(t, err, "creating test store")
	require.NoError(t, s.Close(), "closing test store")
}

func TestOpenRW_DatabaseIsCorrupt(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()
	dbFile := dbLocation(testRootDir)

	// Create corrupt db file
	require.NoError(t, os.WriteFile(dbFile, []byte("not a database"), 0666), "creating corrupt db")

	s, err := OpenRW(t.Context(), testRootDir, StartupSettingsStore)
	require.NoError(t, err, "expected database to be deleted and re-created successfully when corrupt")
	require.NoError(t, s.Close(), "closing test store")
}

func TestOpenRW_DatabaseIsDirty(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()

	// Create the database
	s, err := OpenRW(t.Context(), testRootDir, StartupSettingsStore)
	require.NoError(t, err, "expected no error creating test store")

	// Mark the migration as dirty
	_, err = s.conn.Exec(fmt.Sprintf(`UPDATE %s SET dirty = 1;`, sqlite.DefaultMigrationsTable))
	require.NoError(t, err, "marking migration as dirty")

	// Close the connection
	require.NoError(t, s.Close(), "expected no error closing test store")

	// Open a new connection and expect that it succeeds, forcing the dirty migration
	s2, err := OpenRW(t.Context(), testRootDir, StartupSettingsStore)
	require.NoError(t, err, "expected no error opening test store with dirty migration")
	require.NoError(t, s2.Close(), "expected no error closing test store")

	// Open and close again successfully just to be sure
	s3, err := OpenRW(t.Context(), testRootDir, StartupSettingsStore)
	require.NoError(t, err, "expected no error opening test store with dirty migration")
	require.NoError(t, s3.Close(), "expected no error closing test store")
}

func TestOpenRW_InvalidTable(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()

	_, err := OpenRW(t.Context(), testRootDir, 10001)
	require.Error(t, err, "expected error when passing in table not on allowlist")
}

func TestGetSet(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()

	s, err := OpenRW(t.Context(), testRootDir, StartupSettingsStore)
	require.NoError(t, err, "creating test store")

	flagKey := []byte(keys.UpdateChannel.String())
	flagVal := []byte("beta")

	require.NoError(t, s.Set(flagKey, flagVal), "expected no error setting kv pair")

	returnedVal, err := s.Get(flagKey)
	require.NoError(t, err, "expected no error getting value")
	require.Equal(t, flagVal, returnedVal, "flag value mismatch")

	require.NoError(t, s.Close())
}

func TestUpdate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		updates []map[string]string
		want    map[string]string
	}{
		{
			name:    "empty",
			updates: []map[string]string{{}, {}},
			want:    map[string]string{},
		},
		{
			name:    "single",
			updates: []map[string]string{{"one": "one"}, {"one": "new_one"}},
			want: map[string]string{
				"one": "new_one",
			},
		},
		{
			name: "multiple",
			updates: []map[string]string{
				{
					"one":   "one",
					"two":   "two",
					"three": "three",
				},
				{
					"one":   "new_one",
					"two":   "new_two",
					"three": "new_three",
				},
			},
			want: map[string]string{
				"one":   "new_one",
				"two":   "new_two",
				"three": "new_three",
			},
		},
		{
			name: "delete stale keys",
			updates: []map[string]string{
				{
					"one":   "one",
					"two":   "two",
					"three": "three",
					"four":  "four",
					"five":  "five",
					"six":   "six",
				},
				{
					"four": "four",
				},
			},
			want: map[string]string{
				"four": "four",
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testRootDir := t.TempDir()

			s, err := OpenRW(t.Context(), testRootDir, StartupSettingsStore)
			require.NoError(t, err, "creating test store")

			for _, update := range tt.updates {
				_, err = s.Update(update)
				require.NoError(t, err, "expected no error on update")
			}

			rows, err := s.conn.Query(`SELECT name, value FROM startup_settings;`) //nolint:rowserrcheck // Can't defer rows.Close() AND check rows.Err() in a way that's meaningful in a test
			require.NoError(t, err, "querying kv pairs")
			defer rows.Close()

			existingKeys := make(map[string]bool)
			for rows.Next() {
				var k, v string
				require.NoError(t, rows.Scan(&k, &v), "scanning rows")
				require.Contains(t, tt.want, k, "found key that should have been deleted")
				require.Equal(t, tt.want[k], v, "value mismatch")
				existingKeys[k] = true
			}

			for k := range tt.want {
				require.Contains(t, existingKeys, k, "did not find key that should have been added")
			}

			require.NoError(t, s.Close())
		})
	}
}

func TestSetUpdate_RO(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()

	s, err := OpenRO(t.Context(), multislogger.NewNopLogger(), testRootDir, StartupSettingsStore)
	require.NoError(t, err, "creating test store")

	require.Error(t, s.Set([]byte(keys.UpdateChannel.String()), []byte("beta")), "should not be able to perform set with RO connection")
	_, updateErr := s.Update(map[string]string{"key1": "value1"})
	require.Error(t, updateErr, "should not be able to perform update with RO connection")

	require.NoError(t, s.Close())
}

// Test_Migrations runs all of the migrations in the migrations/ subdirectory
// in both directions, ensuring that all up and down migrations are valid.
func Test_Migrations(t *testing.T) {
	t.Parallel()

	tempRootDir := t.TempDir()

	conn, err := validatedDbConn(t.Context(), tempRootDir)
	require.NoError(t, err, "setting up db connection")
	require.NoError(t, conn.Close(), "closing test db")

	d, err := iofs.New(migrations, "migrations")
	require.NoError(t, err, "loading migration files")

	m, err := migrate.NewWithSourceInstance("iofs", d, fmt.Sprintf("sqlite://%s?query", dbLocation(tempRootDir)))
	require.NoError(t, err, "creating migrate instance")

	require.NoError(t, m.Up(), "expected no error running all migrations")

	require.NoError(t, m.Down(), "expected no error rolling back all migrations")

	srcErr, dbErr := m.Close()
	require.NoError(t, srcErr, "source error closing migration")
	require.NoError(t, dbErr, "database error closing migration")
}

func Test_MissingMigrations(t *testing.T) {
	t.Parallel()

	tempRootDir := t.TempDir()

	conn, err := validatedDbConn(t.Context(), tempRootDir)
	require.NoError(t, err, "setting up db connection")
	require.NoError(t, conn.Close(), "closing test db")

	d, err := iofs.New(migrations, "migrations")
	require.NoError(t, err, "loading migration files")

	m, err := migrate.NewWithSourceInstance("iofs", d, fmt.Sprintf("sqlite://%s?query", dbLocation(tempRootDir)))
	require.NoError(t, err, "creating migrate instance")
	require.NoError(t, m.Up(), "expected no error running all migrations")
	currentVersion, dirty, err := m.Version()
	require.NoError(t, err, "error looking for current version")
	require.False(t, dirty, "did not expect dirty migration state")
	missingMigrationVersion := int(currentVersion) + 1
	forceVersionErr := m.Force(missingMigrationVersion)
	require.NoError(t, forceVersionErr, "error forcing version")

	srcErr, dbErr := m.Close()
	require.NoError(t, srcErr, "source error closing migration")
	require.NoError(t, dbErr, "database error closing migration")

	// now re-open and re-attempt migrations, this will only work if we correctly ignore the missing
	// migration file error
	s, migrationError := OpenRW(t.Context(), tempRootDir, StartupSettingsStore)
	require.NoError(t, migrationError, "database error running missing migration")
	require.NoError(t, s.Close(), "error closing sqliteStore conn")
}
