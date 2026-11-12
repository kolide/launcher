package katc

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/stretchr/testify/require"
)

func Test_sqliteData(t *testing.T) {
	t.Parallel()

	// Set up two sqlite databases with data in them
	sqliteDir := t.TempDir()
	dbFilepaths := []string{
		filepath.Join(sqliteDir, "a.sqlite"),
		filepath.Join(sqliteDir, "b.sqlite"),
	}
	uuids := []string{
		"28f3ebd7-0945-4c54-96af-413a0a0d2dd0",
		"134def6f-b7e2-4ee1-9461-46e54f627835",
	}
	values := []string{
		"value one",
		"value two",
	}
	for i, p := range dbFilepaths {
		// Create file
		f, err := os.Create(p)
		require.NoError(t, err)
		require.NoError(t, f.Close())

		// Open connection
		conn, err := sql.Open("sqlite", p)
		require.NoError(t, err)

		// Create table
		_, err = conn.ExecContext(t.Context(), `
			CREATE TABLE IF NOT EXISTS test_data (
				uuid TEXT NOT NULL PRIMARY KEY,
				value TEXT,
				ignored_column TEXT
			) WITHOUT ROWID;
		`)
		require.NoError(t, err)

		// Add data to table
		_, err = conn.ExecContext(t.Context(), `
			INSERT INTO test_data (uuid, value, ignored_column)
			VALUES (?, ?, "ignored value");
		`, uuids[i], values[i])
		require.NoError(t, err)

		conn.Close()
	}

	// Query data
	results, err := sqliteData(t.Context(), multislogger.NewNopLogger(), []string{filepath.Join(sqliteDir, "*.sqlite")}, "", "SELECT uuid, value FROM test_data;", table.QueryContext{})
	require.NoError(t, err)

	// Confirm we have the correct number of `sourceData` returned (one per db)
	require.Equal(t, 2, len(results))

	// Validate data in each result
	for i, sourceResp := range results {
		// We don't really care about the ordering of results here, but this ensures we can
		// confirm that the data is associated with the correct source
		require.Equal(t, dbFilepaths[i], sourceResp.path)

		// Only one row per source
		require.Equal(t, 1, len(sourceResp.rows))

		// Validate keys and their values
		require.Contains(t, sourceResp.rows[0], "uuid")
		require.Equal(t, uuids[i], string(sourceResp.rows[0]["uuid"]))
		require.Contains(t, sourceResp.rows[0], "value")
		require.Equal(t, values[i], string(sourceResp.rows[0]["value"]))

		// Confirm we didn't pull the other column
		require.NotContains(t, sourceResp.rows[0], "ignored_column")
	}
}

func Test_sqliteData_noSourcesFound(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	results, err := sqliteData(t.Context(), multislogger.NewNopLogger(), []string{filepath.Join(tmpDir, "db.sqlite")}, "idb_cmp1", "SELECT * FROM data;", table.QueryContext{})
	require.NoError(t, err)
	require.Equal(t, 0, len(results))
}

func TestSourcePatternToGlobbablePattern(t *testing.T) {
	t.Parallel()

	// Make actual test directories + files so that we can run filepath.Glob
	rootDir := t.TempDir()
	testDir := filepath.Join(rootDir, "path", "to", "a", "directory")
	require.NoError(t, os.MkdirAll(testDir, 0755))
	dbFile := filepath.Join(testDir, "db.sqlite")
	f, err := os.Create(dbFile)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	for _, tt := range []struct {
		testCaseName    string
		sourcePattern   string
		expectedPattern string
	}{
		{
			testCaseName:    "no wildcards",
			sourcePattern:   filepath.Join(rootDir, "path", "to", "a", "directory", "db.sqlite"),
			expectedPattern: filepath.Join(rootDir, "path", "to", "a", "directory", "db.sqlite"),
		},
		{
			testCaseName:    "% wildcard",
			sourcePattern:   filepath.Join(rootDir, "path", "to", "%", "directory", "db.sqlite"),
			expectedPattern: filepath.Join(rootDir, "path", "to", "*", "directory", "db.sqlite"),
		},
		{
			testCaseName:    "multiple wildcards",
			sourcePattern:   filepath.Join(rootDir, "path", "to", "*", "directory", "%.sqlite"),
			expectedPattern: filepath.Join(rootDir, "path", "to", "*", "directory", "*.sqlite"),
		},
	} {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			// Confirm pattern is as expected
			pattern := sourcePatternToGlobbablePattern(tt.sourcePattern)
			require.Equal(t, tt.expectedPattern, pattern)

			// Confirm pattern is globbable
			matches, err := filepath.Glob(pattern)
			require.NoError(t, err)
			require.Equal(t, 1, len(matches))
			require.Equal(t, dbFile, matches[0])
		})
	}
}

// querySqliteDb should clean up copied files on failure.
func Test_querySqliteDb_cleansUpTempDirOnCopyFailure(t *testing.T) {
	var (
		srcRoot = t.TempDir() // distinct src
		source  = filepath.Join(srcRoot, "missing.sqlite")
		dstRoot = t.TempDir() // distinct dst
	)

	// os.TempDir() for all platforms
	t.Setenv("TMPDIR", dstRoot)
	t.Setenv("TMP", dstRoot)

	_, err := querySqliteDb(t.Context(), multislogger.NewNopLogger(), source, "SELECT 1;")
	require.Error(t, err, "database copy should fail")

	entries, err := os.ReadDir(dstRoot)
	require.NoError(t, err)
	require.Empty(t, entries, "destination still has folders created from the copy")
}

// querySqliteDb should not disturb source files.
func Test_querySqliteDb_leavesSourceIntact(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name       string
		unreadable string
	}{
		{name: "success"},
		{name: "db", unreadable: "db.sqlite"},
		{name: "wal", unreadable: "db.sqlite-wal"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sourceDir := t.TempDir()
			for _, name := range []string{"db.sqlite", "db.sqlite-wal", "db.sqlite-shm", "sibling.txt"} {
				switch name {
				case tt.unreadable:
					require.NoError(t, os.Mkdir(filepath.Join(sourceDir, name), 0755)) // folder is not a db
					continue
				case "db.sqlite":
					conn, err := sql.Open("sqlite", filepath.Join(sourceDir, name))
					require.NoError(t, err)
					_, err = conn.ExecContext(t.Context(), "CREATE TABLE test_data (value TEXT);")
					require.NoError(t, err)
					require.NoError(t, conn.Close())
					continue
				}
				require.NoError(t, os.WriteFile(filepath.Join(sourceDir, name), []byte(name), 0644))
			}
			before := snapshotDir(t, sourceDir)

			_, err := querySqliteDb(t.Context(), multislogger.NewNopLogger(), filepath.Join(sourceDir, "db.sqlite"), "SELECT * FROM test_data;")
			if tt.unreadable == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}

			require.Equal(t, before, snapshotDir(t, sourceDir), "the source directory changed after calling querySqliteDb")
		})
	}
}

// Returns the dir's files mapped to their content's sha.
func snapshotDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	snapshot := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			snapshot[e.Name()] = "<dir>"
			continue
		}
		contents, err := os.ReadFile(filepath.Join(dir, e.Name()))
		require.NoError(t, err)
		h := sha256.New()
		_, err = h.Write(contents)
		require.NoError(t, err)
		snapshot[e.Name()] = hex.EncodeToString(h.Sum(nil))
	}
	return snapshot
}
