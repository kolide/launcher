package katc

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/golang/snappy"
	"github.com/google/uuid"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"
)

func Test_generate_SqliteBackedIndexedDB(t *testing.T) {
	t.Parallel()

	// This test validates generation of table results. It uses a sqlite-backed
	// IndexedDB as a source, which means it also exercises functionality from
	// sqlite.go, snappy.go, and deserialize_firefox.go.

	// First, set up the data we expect to retrieve.
	expectedColumn := "uuid"
	u, err := uuid.NewRandom()
	require.NoError(t, err, "generating test UUID")
	expectedColumnValue := u.String()

	// Serialize the row data, reversing the deserialization operation in
	// deserialize_firefox.go.
	serializedUuid := []byte(expectedColumnValue)
	serializedObj := append([]byte{
		// Header
		0x00, 0x00, 0x00, 0x00, // header tag data -- discarded
		0x00, 0x00, 0xf1, 0xff, // LE `tagHeader`
		// Begin object
		0x00, 0x00, 0x00, 0x00, // object tag data -- discarded
		0x08, 0x00, 0xff, 0xff, // LE `tagObject`
		// Begin UUID key
		0x04, 0x00, 0x00, 0x80, // LE data about upcoming string: length 4 (remaining bytes), is ASCII
		0x04, 0x00, 0xff, 0xff, // LE `tagString`
		0x75, 0x75, 0x69, 0x64, // "uuid"
		0x00, 0x00, 0x00, 0x00, // padding to get to 8-byte word boundary
		// End UUID key
		// Begin UUID value
		0x24, 0x00, 0x00, 0x80, // LE data about upcoming string: length 36 (remaining bytes), is ASCII
		0x04, 0x00, 0xff, 0xff, // LE `tagString`
	},
		serializedUuid...,
	)
	serializedObj = append(serializedObj,
		0x00, 0x00, 0x00, 0x00, // padding to get to 8-byte word boundary for UUID string
		// End UUID value
		0x00, 0x00, 0x00, 0x00, // tag data -- discarded
		0x13, 0x00, 0xff, 0xff, // LE `tagEndOfKeys` 0xffff0013
	)

	// Now compress the serialized row data, reversing the decompression operation
	// in snappy.go
	compressedObj := snappy.Encode(nil, serializedObj)

	// Now, create a sqlite database to store this data in.
	databaseDir := t.TempDir()
	sourceFilepath := filepath.Join(databaseDir, "test.sqlite")
	f, err := os.Create(sourceFilepath)
	require.NoError(t, err, "creating source db")
	require.NoError(t, f.Close(), "closing source db file")
	conn, err := sql.Open("sqlite", sourceFilepath)
	require.NoError(t, err)
	_, err = conn.Exec(`CREATE TABLE object_data(data TEXT NOT NULL PRIMARY KEY) WITHOUT ROWID;`)
	require.NoError(t, err, "creating test table")

	// Insert compressed object into the database
	_, err = conn.Exec("INSERT INTO object_data (data) VALUES (?);", compressedObj)
	require.NoError(t, err, "inserting into sqlite database")
	require.NoError(t, conn.Close(), "closing sqlite database")

	// At long last, our source is adequately configured.
	// Move on to constructing our KATC table.
	cfg := katcTableConfig{
		SourceType: katcSourceType{
			name:     sqliteSourceType,
			dataFunc: sqliteData,
		},
		Platform: runtime.GOOS,
		Columns:  []string{expectedColumn},
		Source:   filepath.Join(databaseDir, "%.sqlite"), // All sqlite files in the test directory
		Query:    "SELECT data FROM object_data;",
		RowTransformSteps: []rowTransformStep{
			{
				name:          snappyDecodeTransformStep,
				transformFunc: snappyDecode,
			},
			{
				name:          deserializeFirefoxTransformStep,
				transformFunc: deserializeFirefox,
			},
		},
	}
	testTable, _ := newKatcTable("test_katc_table", cfg, multislogger.NewNopLogger())

	// Make a query context restricting the source to our exact source sqlite database
	queryContext := table.QueryContext{
		Constraints: map[string]table.ConstraintList{
			pathColumnName: {
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorEquals,
						Expression: sourceFilepath,
					},
				},
			},
		},
	}

	// At long last: run a query
	results, err := testTable.generate(context.TODO(), queryContext)
	require.NoError(t, err)

	// Validate results
	require.Equal(t, 1, len(results), "exactly one row expected")
	require.Contains(t, results[0], pathColumnName, "missing source column")
	require.Equal(t, sourceFilepath, results[0][pathColumnName])
	require.Contains(t, results[0], expectedColumn, "expected column missing")
	require.Equal(t, expectedColumnValue, results[0][expectedColumn], "data mismatch")
}

func Test_checkSourcePathConstraints(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName  string
		path          string
		constraints   table.ConstraintList
		valid         bool
		errorExpected bool
	}{
		{
			testCaseName: "equals",
			path:         filepath.Join("some", "path", "to", "a", "source"),
			constraints: table.ConstraintList{
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorEquals,
						Expression: filepath.Join("some", "path", "to", "a", "source"),
					},
				},
			},
			valid:         true,
			errorExpected: false,
		},
		{
			testCaseName: "not equals",
			path:         filepath.Join("some", "path", "to", "a", "source"),
			constraints: table.ConstraintList{
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorEquals,
						Expression: filepath.Join("a", "path", "to", "a", "different", "source"),
					},
				},
			},
			valid:         false,
			errorExpected: false,
		},
		{
			testCaseName: "LIKE with % wildcard",
			path:         filepath.Join("a", "path", "to", "db.sqlite"),
			constraints: table.ConstraintList{
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorLike,
						Expression: filepath.Join("a", "path", "to", "%.sqlite"),
					},
				},
			},
			valid:         true,
			errorExpected: false,
		},
		{
			testCaseName: "LIKE with underscore wildcard",
			path:         filepath.Join("a", "path", "to", "db.sqlite"),
			constraints: table.ConstraintList{
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorLike,
						Expression: filepath.Join("_", "path", "to", "db.sqlite"),
					},
				},
			},
			valid:         true,
			errorExpected: false,
		},
		{
			testCaseName: "LIKE is case-insensitive",
			path:         filepath.Join("a", "path", "to", "db.sqlite"),
			constraints: table.ConstraintList{
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorLike,
						Expression: filepath.Join("A", "PATH", "TO", "DB.%"),
					},
				},
			},
			valid: true,
		},
		{
			testCaseName: "GLOB with * wildcard",
			path:         filepath.Join("another", "path", "to", "a", "source"),
			constraints: table.ConstraintList{
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorGlob,
						Expression: filepath.Join("another", "*", "to", "a", "source"),
					},
				},
			},
			valid:         true,
			errorExpected: false,
		},
		{
			testCaseName: "GLOB with ? wildcard",
			path:         filepath.Join("another", "path", "to", "a", "source"),
			constraints: table.ConstraintList{
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorGlob,
						Expression: filepath.Join("another", "path", "to", "?", "source"),
					},
				},
			},
			valid:         true,
			errorExpected: false,
		},
		{
			testCaseName: "regexp",
			path:         filepath.Join("test", "path", "to", "a", "source"),
			constraints: table.ConstraintList{
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorRegexp,
						Expression: `.*source$`,
					},
				},
			},
			valid:         true,
			errorExpected: false,
		},
		{
			testCaseName: "invalid regexp",
			path:         filepath.Join("test", "path", "to", "a", "source"),
			constraints: table.ConstraintList{
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorRegexp,
						Expression: `invalid\`,
					},
				},
			},
			valid:         false,
			errorExpected: true,
		},
		{
			testCaseName: "unsupported",
			path:         filepath.Join("test", "path", "to", "a", "source", "2"),
			constraints: table.ConstraintList{
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorUnique,
						Expression: filepath.Join("test", "path", "to", "a", "source", "2"),
					},
				},
			},
			valid:         false,
			errorExpected: true,
		},
		{
			testCaseName: "multiple constraints where one does not match",
			path:         filepath.Join("test", "path", "to", "a", "source", "3"),
			constraints: table.ConstraintList{
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorLike,
						Expression: filepath.Join("test", "path", "to", "a", "source", "%"),
					},
					{
						Operator:   table.OperatorEquals,
						Expression: filepath.Join("some", "path", "to", "a", "source"),
					},
				},
			},
			valid:         false,
			errorExpected: false,
		},
		{
			testCaseName: "multiple constraints where all match",
			path:         filepath.Join("test", "path", "to", "a", "source", "3"),
			constraints: table.ConstraintList{
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorLike,
						Expression: filepath.Join("test", "path", "to", "a", "source", "%"),
					},
					{
						Operator:   table.OperatorEquals,
						Expression: filepath.Join("test", "path", "to", "a", "source", "3"),
					},
				},
			},
			valid:         true,
			errorExpected: false,
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			valid, err := checkPathConstraints(tt.path, &tt.constraints)
			if tt.errorExpected {
				require.Error(t, err, "expected error on checking constraints")
			} else {
				require.NoError(t, err, "expected no error on checking constraints")
			}

			require.Equal(t, tt.valid, valid, "incorrect result checking constraints")
		})
	}
}
