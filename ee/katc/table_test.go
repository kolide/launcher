package katc

import (
	"archive/zip"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/kolide/goleveldb/leveldb/opt"
	"github.com/kolide/launcher/ee/indexeddb"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"
)

//go:embed test_data/indexeddbs/1985929987lbadutnscehter.sqlite.zip
var basicFirefoxIndexeddb []byte

//go:embed test_data/indexeddbs/file__0.indexeddb.leveldb.zip
var basicChromeIndexeddb []byte

func TestQueryFirefoxIndexedDB(t *testing.T) {
	t.Parallel()

	// This test validates generation of table results. It uses a sqlite-backed
	// IndexedDB as a source, which means it also exercises functionality from
	// sqlite.go, snappy.go, and deserialize_firefox.go.

	for _, tt := range []struct {
		fileName     string
		objStoreName string
		expectedRows int
		zipBytes     []byte
	}{
		{
			fileName:     "1985929987lbadutnscehter.sqlite.zip",
			objStoreName: "launchertestobjstore",
			expectedRows: 2,
			zipBytes:     basicFirefoxIndexeddb,
		},
	} {
		tt := tt
		t.Run(tt.fileName, func(t *testing.T) {
			t.Parallel()

			// Write zip bytes to file
			tempDir := t.TempDir()
			zipFile := filepath.Join(tempDir, tt.fileName)
			require.NoError(t, os.WriteFile(zipFile, tt.zipBytes, 0755), "writing zip to temp dir")

			// Unzip to file in temp dir
			indexeddbDest := strings.TrimSuffix(zipFile, ".zip")
			zipReader, err := zip.OpenReader(zipFile)
			require.NoError(t, err, "opening reader to zip file")
			defer zipReader.Close()
			for _, fileInZip := range zipReader.File {
				unzipFile(t, fileInZip, tempDir)
			}

			// Construct table
			sourceQuery := fmt.Sprintf("SELECT data FROM object_data JOIN object_store ON (object_data.object_store_id = object_store.id) WHERE object_store.name=\"%s\";", tt.objStoreName)
			cfg := katcTableConfig{
				Columns: []string{
					"uuid", "name", "version", "preferences", "flags", "aliases",
					"linkedIds", "anotherSparseArray", "someDetails", "noDetails",
					"numArray", "email", "someTimestamp", "someDate", "someMap",
					"someComplexMap", "someSet", "someRegex", "someStringObject",
					"someNumberObject", "someDouble", "someBoolean", "someTypedArray",
					"someArrayBuffer", "anotherTypedArray", "yetAnotherTypedArray",
					"basicError", "evalError", "rangeError", "referenceError",
					"syntaxError", "typeError", "uriError", "errorWithCause",
				},
				katcTableDefinition: katcTableDefinition{
					SourceType: &katcSourceType{
						name:     sqliteSourceType,
						dataFunc: sqliteData,
					},
					SourcePaths: &[]string{filepath.Join("some", "incorrect", "path")},
					SourceQuery: &sourceQuery,
					RowTransformSteps: &[]rowTransformStep{
						{
							name:          snappyDecodeTransformStep,
							transformFunc: snappyDecode,
						},
						{
							name:          deserializeFirefoxTransformStep,
							transformFunc: deserializeFirefox,
						},
					},
				},
				Overlays: []katcTableConfigOverlay{
					{
						Filters: map[string]string{
							"goos": runtime.GOOS,
						},
						katcTableDefinition: katcTableDefinition{
							SourcePaths: &[]string{indexeddbDest}, // All sqlite files in the test directory
						},
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
								Expression: indexeddbDest,
							},
						},
					},
				},
			}

			// At long last: run a query
			results, err := testTable.generate(t.Context(), queryContext)
			require.NoError(t, err)

			// We should have the expected number of results in the row
			require.Equal(t, tt.expectedRows, len(results), "unexpected number of rows returned")

			// In the TestQueryFirefoxIndexedDB function, add these require statements inside the for loop that checks columns
			for i := 0; i < tt.expectedRows; i += 1 {
				require.Contains(t, results[i], pathColumnName, "missing source column")
				require.Equal(t, indexeddbDest, results[i][pathColumnName])
				require.Contains(t, results[i], "uuid", "expected uuid column missing")
				require.Contains(t, results[i], "name", "expected name column missing")
				require.Contains(t, results[i], "version", "expected version column missing")
				require.Contains(t, results[i], "preferences", "expected preferences column missing")
				require.Contains(t, results[i], "flags", "expected flags column missing")
				require.Contains(t, results[i], "aliases", "expected aliases column missing")
				require.Contains(t, results[i], "linkedIds", "expected linkedIds column missing")
				require.Contains(t, results[i], "anotherSparseArray", "expected anotherSparseArray column missing")
				require.Contains(t, results[i], "someDetails", "expected someDetails column missing")
				require.Contains(t, results[i], "noDetails", "expected noDetails column missing")
				require.Contains(t, results[i], "numArray", "expected numArray column missing")
				require.Contains(t, results[i], "email", "expected email column missing")
				require.Contains(t, results[i], "someTimestamp", "expected someTimestamp column missing")
				require.Contains(t, results[i], "someDate", "expected someDate column missing")
				require.Contains(t, results[i], "someMap", "expected someMap column missing")
				require.Contains(t, results[i], "someComplexMap", "expected someComplexMap column missing")
				require.Contains(t, results[i], "someRegex", "expected someRegex column missing")
				require.Contains(t, results[i], "someStringObject", "expected someStringObject column missing")
				require.Contains(t, results[i], "someNumberObject", "expected someNumberObject column missing")
				require.Contains(t, results[i], "someDouble", "expected someDouble column missing")
				require.Contains(t, results[i], "someBoolean", "expected someBoolean column missing")
				require.Contains(t, results[i], "someTypedArray", "expected someTypedArray column missing")
				require.Contains(t, results[i], "someArrayBuffer", "expected someArrayBuffer column missing")
				require.Contains(t, results[i], "anotherTypedArray", "expected anotherTypedArray column missing")
				require.Contains(t, results[i], "yetAnotherTypedArray", "expected yetAnotherTypedArray column missing")
				require.Contains(t, results[i], "basicError", "expected basicError column missing")
				require.Contains(t, results[i], "evalError", "expected evalError column missing")
				require.Contains(t, results[i], "rangeError", "expected rangeError column missing")
				require.Contains(t, results[i], "referenceError", "expected referenceError column missing")
				require.Contains(t, results[i], "syntaxError", "expected syntaxError column missing")
				require.Contains(t, results[i], "typeError", "expected typeError column missing")
				require.Contains(t, results[i], "uriError", "expected uriError column missing")
				require.Contains(t, results[i], "errorWithCause", "expected errorWithCause column missing")
			}
		})
	}
}

func unzipFile(t *testing.T, fileInZip *zip.File, tempDir string) {
	fileInZipReader, err := fileInZip.Open()
	require.NoError(t, err, "opening file in zip")
	defer fileInZipReader.Close()

	idbFilePath := filepath.Join(tempDir, fileInZip.Name)

	if fileInZip.FileInfo().IsDir() {
		require.NoError(t, os.MkdirAll(idbFilePath, fileInZip.Mode()), "creating dir")
		return
	}

	outFile, err := os.OpenFile(idbFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fileInZip.Mode())
	require.NoError(t, err, "opening output file")
	defer outFile.Close()

	_, err = io.Copy(outFile, fileInZipReader)
	require.NoError(t, err, "copying from zip to temp dir")
}

func TestQueryChromeIndexedDB(t *testing.T) {
	t.Parallel()

	// This test validates generation of table results. It uses a leveldb-backed
	// IndexedDB as a source, which means it also exercises functionality from
	// indexeddb_leveldb.go and the ee/indexeddb package.

	for _, tt := range []struct {
		testName     string
		fileName     string
		dbName       string
		objStoreName string
		expectedRows int
		zipBytes     []byte
	}{
		{
			testName:     "file__0.indexeddb.leveldb.zip",
			fileName:     "file__0.indexeddb.leveldb.zip",
			dbName:       "launchertestdb",
			objStoreName: "launchertestobjstore",
			expectedRows: 2,
			zipBytes:     basicChromeIndexeddb,
		},
		{
			testName:     "file__0.indexeddb.leveldb.zip -- db does not exist",
			fileName:     "file__0.indexeddb.leveldb.zip",
			dbName:       "not-the-correct-db-name",
			objStoreName: "launchertestobjstore",
			expectedRows: 0,
			zipBytes:     basicChromeIndexeddb,
		},
		{
			testName:     "file__0.indexeddb.leveldb.zip -- object store does not exist",
			fileName:     "file__0.indexeddb.leveldb.zip",
			dbName:       "launchertestdb",
			objStoreName: "not-the-correct-obj-store-name",
			expectedRows: 0,
			zipBytes:     basicChromeIndexeddb,
		},
	} {
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()

			// Write zip bytes to file
			tempDir := t.TempDir()
			zipFile := filepath.Join(tempDir, tt.fileName)
			require.NoError(t, os.WriteFile(zipFile, tt.zipBytes, 0755), "writing zip to temp dir")

			// Prepare indexeddb dir
			indexeddbDest := strings.TrimSuffix(zipFile, ".zip")
			require.NoError(t, os.MkdirAll(indexeddbDest, 0755), "creating indexeddb dir")

			// Unzip to temp dir
			zipReader, err := zip.OpenReader(zipFile)
			require.NoError(t, err, "opening reader to zip file")
			defer zipReader.Close()
			for _, fileInZip := range zipReader.File {
				unzipFile(t, fileInZip, tempDir)
			}

			// Construct table
			sourceQuery := fmt.Sprintf("%s.%s", tt.dbName, tt.objStoreName)
			cfg := katcTableConfig{
				Columns: []string{
					"uuid", "name", "version", "preferences", "flags", "aliases",
					"linkedIds", "anotherSparseArray", "someDetails", "noDetails",
					"numArray", "email", "someTimestamp", "someDate", "someMap",
					"someComplexMap", "someSet", "someRegex", "someStringObject",
					"someNumberObject", "someDouble", "someBoolean", "someTypedArray",
					"someArrayBuffer", "anotherTypedArray", "yetAnotherTypedArray",
					"basicError", "evalError", "rangeError", "referenceError",
					"syntaxError", "typeError", "uriError", "errorWithCause",
				},
				katcTableDefinition: katcTableDefinition{
					SourceType: &katcSourceType{
						name:     indexeddbLeveldbSourceType,
						dataFunc: indexeddbLeveldbData,
					},
					SourcePaths: &[]string{filepath.Join("some", "incorrect", "path")},
					SourceQuery: &sourceQuery,
					RowTransformSteps: &[]rowTransformStep{
						{
							name:          deserializeChromeTransformStep,
							transformFunc: indexeddb.DeserializeChrome,
						},
					},
				},
				Overlays: []katcTableConfigOverlay{
					{
						Filters: map[string]string{
							"goos": runtime.GOOS,
						},
						katcTableDefinition: katcTableDefinition{
							SourcePaths: &[]string{indexeddbDest}, // All indexeddb files in the test directory
						},
					},
				},
			}
			testTable, _ := newKatcTable("test_katc_table", cfg, multislogger.NewNopLogger())

			// Make a query context restricting the source to our exact source indexeddb database
			queryContext := table.QueryContext{
				Constraints: map[string]table.ConstraintList{
					pathColumnName: {
						Constraints: []table.Constraint{
							{
								Operator:   table.OperatorEquals,
								Expression: indexeddbDest,
							},
						},
					},
				},
			}

			// At long last: run a query
			results, err := testTable.generate(t.Context(), queryContext)
			require.NoError(t, err)

			// We should have the expected number of results in the row
			require.Equal(t, tt.expectedRows, len(results), "unexpected number of rows returned")

			// Make sure we have the expected number of columns
			for i := 0; i < tt.expectedRows; i += 1 {
				require.Contains(t, results[i], pathColumnName, "missing source column")
				require.Equal(t, indexeddbDest, results[i][pathColumnName])
				require.Contains(t, results[i], "uuid", "expected uuid column missing")
				require.Contains(t, results[i], "name", "expected name column missing")
				require.Contains(t, results[i], "version", "expected version column missing")
				require.Contains(t, results[i], "preferences", "expected preferences column missing")
				require.Contains(t, results[i], "flags", "expected flags column missing")
				require.Contains(t, results[i], "aliases", "expected aliases column missing")
				require.Contains(t, results[i], "linkedIds", "expected linkedIds column missing")
				require.Contains(t, results[i], "anotherSparseArray", "expected anotherSparseArray column missing")
				require.Contains(t, results[i], "someDetails", "expected someDetails column missing")
				require.Contains(t, results[i], "noDetails", "expected noDetails column missing")
				require.Contains(t, results[i], "numArray", "expected numArray column missing")
				require.Contains(t, results[i], "email", "expected email column missing")
				require.Contains(t, results[i], "someTimestamp", "expected someTimestamp column missing")
				require.Contains(t, results[i], "someDate", "expected someDate column missing")
				require.Contains(t, results[i], "someMap", "expected someMap column missing")
				require.Contains(t, results[i], "someComplexMap", "expected someComplexMap column missing")
				require.Contains(t, results[i], "someRegex", "expected someRegex column missing")
				require.Contains(t, results[i], "someStringObject", "expected someStringObject column missing")
				require.Contains(t, results[i], "someNumberObject", "expected someNumberObject column missing")
				require.Contains(t, results[i], "someDouble", "expected someDouble column missing")
				require.Contains(t, results[i], "someBoolean", "expected someBoolean column missing")
				require.Contains(t, results[i], "someTypedArray", "expected someTypedArray column missing")
				require.Contains(t, results[i], "someArrayBuffer", "expected someArrayBuffer column missing")
				require.Contains(t, results[i], "anotherTypedArray", "expected anotherTypedArray column missing")
				require.Contains(t, results[i], "yetAnotherTypedArray", "expected yetAnotherTypedArray column missing")
				require.Contains(t, results[i], "basicError", "expected basicError column missing")
				require.Contains(t, results[i], "evalError", "expected evalError column missing")
				require.Contains(t, results[i], "rangeError", "expected rangeError column missing")
				require.Contains(t, results[i], "referenceError", "expected referenceError column missing")
				require.Contains(t, results[i], "syntaxError", "expected syntaxError column missing")
				require.Contains(t, results[i], "typeError", "expected typeError column missing")
				require.Contains(t, results[i], "uriError", "expected uriError column missing")
				require.Contains(t, results[i], "errorWithCause", "expected errorWithCause column missing")
			}
		})
	}
}

func TestQueryChromeIndexedDBMixedKeyIteration(t *testing.T) {
	t.Parallel()

	levelDBZipFileName := "file__0.indexeddb.leveldb.zip"
	// Write zip bytes to file
	tempDir := t.TempDir()
	zipFile := filepath.Join(tempDir, levelDBZipFileName)
	require.NoError(t, os.WriteFile(zipFile, basicChromeIndexeddb, 0755), "writing zip to temp dir")

	// Prepare indexeddb dir
	indexeddbDest := strings.TrimSuffix(zipFile, ".zip")
	require.NoError(t, os.MkdirAll(indexeddbDest, 0755), "creating indexeddb dir")

	// Unzip to temp dir
	zipReader, err := zip.OpenReader(zipFile)
	require.NoError(t, err, "opening reader to zip file")
	defer zipReader.Close()
	for _, fileInZip := range zipReader.File {
		unzipFile(t, fileInZip, tempDir)
	}

	// the mixed keys are in a different object store than the other tests in this file
	databaseName := "launchertestdb"
	objectStoreName := "launchertestobjstore-mixedkeys"
	sourceQuery := fmt.Sprintf("%s.%s", databaseName, objectStoreName)
	// Construct table
	cfg := katcTableConfig{
		Columns: []string{
			"test",
		},
		katcTableDefinition: katcTableDefinition{
			SourceType: &katcSourceType{
				name:     indexeddbLeveldbSourceType,
				dataFunc: indexeddbLeveldbData,
			},
			SourcePaths: &[]string{indexeddbDest},
			SourceQuery: &sourceQuery,
			RowTransformSteps: &[]rowTransformStep{
				{
					name:          deserializeChromeTransformStep,
					transformFunc: indexeddb.DeserializeChrome,
				},
			},
		},
		Overlays: []katcTableConfigOverlay{
			{
				Filters: map[string]string{
					"goos": runtime.GOOS,
				},
				katcTableDefinition: katcTableDefinition{
					SourcePaths: &[]string{indexeddbDest}, // All indexeddb files in the test directory
				},
			},
		},
	}
	testTable, _ := newKatcTable("test_katc_table", cfg, multislogger.NewNopLogger())

	// Make a query context restricting the source to our exact source indexeddb database
	queryContext := table.QueryContext{
		Constraints: map[string]table.ConstraintList{
			pathColumnName: {
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorEquals,
						Expression: indexeddbDest,
					},
				},
			},
		},
	}

	results, err := testTable.generate(t.Context(), queryContext)
	require.NoError(t, err)
	// we expect exactly 9 rows added to the mixed keys object store. see test_data/main.js for details
	// on how this is created with various key types
	require.Equal(t, 9, len(results))

	resultsSeen := make([]bool, len(results))
	// all rows are a simple json object in the format {"test": "<insertionNumber>"}
	// we do not expect these to come out in the order they were inserted (it is done with mixed key types),
	// but we do expect to see a row with each individual number 1-9, ensuring no corruption during iteration
	for _, result := range results {
		require.Contains(t, result, "test")
		insertionNumber, err := strconv.Atoi(result["test"])
		require.NoError(t, err)
		require.Less(t, insertionNumber-1, len(resultsSeen)) // sanity check
		resultsSeen[insertionNumber-1] = true
	}
	// verify that all 9 have been seen
	for i, seen := range resultsSeen {
		require.True(t, seen, "test number %d was not seen during iteration", i)
	}
}

func TestQueryLevelDB(t *testing.T) {
	t.Parallel()

	// Create a level db to query
	tempDir := t.TempDir()
	db, err := indexeddb.OpenLeveldb(multislogger.NewNopLogger(), tempDir)
	require.NoError(t, err)

	// Add some data
	expectedKey := "some-key"
	expectedValue := "some-value"
	require.NoError(t, db.Put([]byte(expectedKey), []byte(expectedValue), &opt.WriteOptions{}))
	require.NoError(t, db.Close())

	// Construct table
	sourceQuery := strings.Join([]string{expectedKey}, ",")
	cfg := katcTableConfig{
		Columns: []string{"key", "value"},
		katcTableDefinition: katcTableDefinition{
			SourceType: &katcSourceType{
				name:     leveldbSourceType,
				dataFunc: leveldbData,
			},
			SourcePaths:       &[]string{filepath.Join("some", "incorrect", "path")},
			SourceQuery:       &sourceQuery,
			RowTransformSteps: &[]rowTransformStep{},
		},
		Overlays: []katcTableConfigOverlay{
			{
				Filters: map[string]string{
					"goos": runtime.GOOS,
				},
				katcTableDefinition: katcTableDefinition{
					SourcePaths: &[]string{tempDir},
				},
			},
		},
	}
	testTable, _ := newKatcTable("test_katc_table", cfg, multislogger.NewNopLogger())

	// Make a query context restricting the source to our exact source indexeddb database
	queryContext := table.QueryContext{
		Constraints: map[string]table.ConstraintList{
			pathColumnName: {
				Constraints: []table.Constraint{
					{
						Operator:   table.OperatorEquals,
						Expression: tempDir,
					},
				},
			},
		},
	}

	// At long last: run a query
	results, err := testTable.generate(t.Context(), queryContext)
	require.NoError(t, err)

	require.Equal(t, 1, len(results))
	require.Contains(t, results[0], "key")
	require.Equal(t, expectedKey, results[0]["key"])
	require.Contains(t, results[0], "value")
	require.Equal(t, expectedValue, results[0]["value"])
	require.Contains(t, results[0], "path")
	require.Equal(t, tempDir, results[0]["path"])
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
