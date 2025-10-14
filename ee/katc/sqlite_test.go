package katc

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/kolide/launcher/pkg/log/multislogger"
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
		_, err = conn.Exec(`
			CREATE TABLE IF NOT EXISTS test_data (
				uuid TEXT NOT NULL PRIMARY KEY,
				value TEXT,
				ignored_column TEXT
			) WITHOUT ROWID;
		`)
		require.NoError(t, err)

		// Add data to table
		_, err = conn.Exec(`
			INSERT INTO test_data (uuid, value, ignored_column)
			VALUES (?, ?, "ignored value");
		`, uuids[i], values[i])
		require.NoError(t, err)

		conn.Close()
	}

	// Query data
	results, err := sqliteData(t.Context(), multislogger.NewNopLogger(), []string{filepath.Join(sqliteDir, "*.sqlite")}, "SELECT uuid, value FROM test_data;", table.QueryContext{})
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
	results, err := sqliteData(t.Context(), multislogger.NewNopLogger(), []string{filepath.Join(tmpDir, "db.sqlite")}, "SELECT * FROM data;", table.QueryContext{})
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
		tt := tt
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
