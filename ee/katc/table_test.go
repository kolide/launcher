package katc

import (
	"archive/zip"
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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
				Columns: []string{"uuid", "name", "version"},
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
			results, err := testTable.generate(context.TODO(), queryContext)
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
				Columns: []string{"uuid", "name", "version"},
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
			results, err := testTable.generate(context.TODO(), queryContext)
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
			}
		})
	}
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

func TestEquals(t *testing.T) {
	t.Parallel()

	testSourceQueryA := "test query a"
	testSourceQueryB := "test query b"

	for _, tt := range []struct {
		testCaseName   string
		tableNameA     string
		cfgA           katcTableConfig
		tableNameB     string
		cfgB           katcTableConfig
		expectedEquals bool
	}{
		{
			testCaseName: "configurations match",
			tableNameA:   "test_table",
			cfgA: katcTableConfig{
				Columns: []string{"uuid", "name", "version"},
				katcTableDefinition: katcTableDefinition{
					SourceType: &katcSourceType{
						name:     sqliteSourceType,
						dataFunc: sqliteData,
					},
					SourcePaths: &[]string{filepath.Join("some", "incorrect", "path")},
					SourceQuery: &testSourceQueryA,
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
			},
			tableNameB: "test_table",
			cfgB: katcTableConfig{
				Columns: []string{"uuid", "name", "version"},
				katcTableDefinition: katcTableDefinition{
					SourceType: &katcSourceType{
						name:     sqliteSourceType,
						dataFunc: sqliteData,
					},
					SourcePaths: &[]string{filepath.Join("some", "incorrect", "path")},
					SourceQuery: &testSourceQueryA,
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
			},
			expectedEquals: true,
		},
		{
			testCaseName: "configurations match with overlays",
			tableNameA:   "test_table",
			cfgA: katcTableConfig{
				Columns: []string{"uuid", "name", "version"},
				katcTableDefinition: katcTableDefinition{
					SourceType: &katcSourceType{
						name:     sqliteSourceType,
						dataFunc: sqliteData,
					},
					SourcePaths: &[]string{filepath.Join("some", "incorrect", "path")},
					SourceQuery: &testSourceQueryA,
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
							SourcePaths: &[]string{filepath.Join("some", "correct", "path")},
						},
					},
				},
			},
			tableNameB: "test_table",
			cfgB: katcTableConfig{
				Columns: []string{"uuid", "name", "version"},
				katcTableDefinition: katcTableDefinition{
					SourceType: &katcSourceType{
						name:     sqliteSourceType,
						dataFunc: sqliteData,
					},
					SourcePaths: &[]string{filepath.Join("some", "incorrect", "path")},
					SourceQuery: &testSourceQueryA,
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
							SourcePaths: &[]string{filepath.Join("some", "correct", "path")},
						},
					},
				},
			},
			expectedEquals: true,
		},
		{
			testCaseName: "table names differ",
			tableNameA:   "test_table",
			cfgA: katcTableConfig{
				Columns: []string{"uuid", "name", "version"},
				katcTableDefinition: katcTableDefinition{
					SourceType: &katcSourceType{
						name:     sqliteSourceType,
						dataFunc: sqliteData,
					},
					SourcePaths: &[]string{filepath.Join("some", "incorrect", "path")},
					SourceQuery: &testSourceQueryA,
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
			},
			tableNameB: "different_test_table",
			cfgB: katcTableConfig{
				Columns: []string{"uuid", "name", "version"},
				katcTableDefinition: katcTableDefinition{
					SourceType: &katcSourceType{
						name:     sqliteSourceType,
						dataFunc: sqliteData,
					},
					SourcePaths: &[]string{filepath.Join("some", "incorrect", "path")},
					SourceQuery: &testSourceQueryA,
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
			},
			expectedEquals: false,
		},
		{
			testCaseName: "source types differ",
			tableNameA:   "test_table",
			cfgA: katcTableConfig{
				Columns: []string{"uuid", "name", "version"},
				katcTableDefinition: katcTableDefinition{
					SourceType: &katcSourceType{
						name:     indexeddbLeveldbSourceType,
						dataFunc: indexeddbLeveldbData,
					},
					SourcePaths: &[]string{filepath.Join("some", "incorrect", "path")},
					SourceQuery: &testSourceQueryA,
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
			},
			tableNameB: "test_table",
			cfgB: katcTableConfig{
				Columns: []string{"uuid", "name", "version"},
				katcTableDefinition: katcTableDefinition{
					SourceType: &katcSourceType{
						name:     sqliteSourceType,
						dataFunc: sqliteData,
					},
					SourcePaths: &[]string{filepath.Join("some", "incorrect", "path")},
					SourceQuery: &testSourceQueryA,
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
			},
			expectedEquals: false,
		},
		{
			testCaseName: "source queries differ",
			tableNameA:   "test_table",
			cfgA: katcTableConfig{
				Columns: []string{"uuid", "name", "version"},
				katcTableDefinition: katcTableDefinition{
					SourceType: &katcSourceType{
						name:     sqliteSourceType,
						dataFunc: sqliteData,
					},
					SourcePaths: &[]string{filepath.Join("some", "incorrect", "path")},
					SourceQuery: &testSourceQueryA,
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
			},
			tableNameB: "test_table",
			cfgB: katcTableConfig{
				Columns: []string{"uuid", "name", "version"},
				katcTableDefinition: katcTableDefinition{
					SourceType: &katcSourceType{
						name:     sqliteSourceType,
						dataFunc: sqliteData,
					},
					SourcePaths: &[]string{filepath.Join("some", "incorrect", "path")},
					SourceQuery: &testSourceQueryB,
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
			},
			expectedEquals: false,
		},
		{
			testCaseName: "source paths differ",
			tableNameA:   "test_table",
			cfgA: katcTableConfig{
				Columns: []string{"uuid", "name", "version"},
				katcTableDefinition: katcTableDefinition{
					SourceType: &katcSourceType{
						name:     sqliteSourceType,
						dataFunc: sqliteData,
					},
					SourcePaths: &[]string{filepath.Join("some", "incorrect", "path")},
					SourceQuery: &testSourceQueryA,
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
			},
			tableNameB: "test_table",
			cfgB: katcTableConfig{
				Columns: []string{"uuid", "name", "version"},
				katcTableDefinition: katcTableDefinition{
					SourceType: &katcSourceType{
						name:     sqliteSourceType,
						dataFunc: sqliteData,
					},
					SourcePaths: &[]string{filepath.Join("some", "other", "path")},
					SourceQuery: &testSourceQueryA,
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
			},
			expectedEquals: false,
		},
		{
			testCaseName: "row transform steps differ in length",
			tableNameA:   "test_table",
			cfgA: katcTableConfig{
				Columns: []string{"uuid", "name", "version"},
				katcTableDefinition: katcTableDefinition{
					SourceType: &katcSourceType{
						name:     sqliteSourceType,
						dataFunc: sqliteData,
					},
					SourcePaths: &[]string{filepath.Join("some", "incorrect", "path")},
					SourceQuery: &testSourceQueryA,
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
			},
			tableNameB: "test_table",
			cfgB: katcTableConfig{
				Columns: []string{"uuid", "name", "version"},
				katcTableDefinition: katcTableDefinition{
					SourceType: &katcSourceType{
						name:     sqliteSourceType,
						dataFunc: sqliteData,
					},
					SourcePaths: &[]string{filepath.Join("some", "incorrect", "path")},
					SourceQuery: &testSourceQueryA,
					RowTransformSteps: &[]rowTransformStep{
						{
							name:          snappyDecodeTransformStep,
							transformFunc: snappyDecode,
						},
					},
				},
			},
			expectedEquals: false,
		},
		{
			testCaseName: "row transform steps differ in order",
			tableNameA:   "test_table",
			cfgA: katcTableConfig{
				Columns: []string{"uuid", "name", "version"},
				katcTableDefinition: katcTableDefinition{
					SourceType: &katcSourceType{
						name:     sqliteSourceType,
						dataFunc: sqliteData,
					},
					SourcePaths: &[]string{filepath.Join("some", "incorrect", "path")},
					SourceQuery: &testSourceQueryA,
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
			},
			tableNameB: "test_table",
			cfgB: katcTableConfig{
				Columns: []string{"uuid", "name", "version"},
				katcTableDefinition: katcTableDefinition{
					SourceType: &katcSourceType{
						name:     sqliteSourceType,
						dataFunc: sqliteData,
					},
					SourcePaths: &[]string{filepath.Join("some", "incorrect", "path")},
					SourceQuery: &testSourceQueryA,
					RowTransformSteps: &[]rowTransformStep{
						{
							name:          deserializeFirefoxTransformStep,
							transformFunc: deserializeFirefox,
						},
						{
							name:          snappyDecodeTransformStep,
							transformFunc: snappyDecode,
						},
					},
				},
			},
			expectedEquals: false,
		},
		{
			testCaseName: "columns differ in length",
			tableNameA:   "test_table",
			cfgA: katcTableConfig{
				Columns: []string{"uuid", "name", "version"},
				katcTableDefinition: katcTableDefinition{
					SourceType: &katcSourceType{
						name:     sqliteSourceType,
						dataFunc: sqliteData,
					},
					SourcePaths: &[]string{filepath.Join("some", "incorrect", "path")},
					SourceQuery: &testSourceQueryA,
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
			},
			tableNameB: "test_table",
			cfgB: katcTableConfig{
				Columns: []string{"name", "version"},
				katcTableDefinition: katcTableDefinition{
					SourceType: &katcSourceType{
						name:     sqliteSourceType,
						dataFunc: sqliteData,
					},
					SourcePaths: &[]string{filepath.Join("some", "incorrect", "path")},
					SourceQuery: &testSourceQueryA,
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
			},
			expectedEquals: false,
		},
		{
			testCaseName: "columns differ in content",
			tableNameA:   "test_table",
			cfgA: katcTableConfig{
				Columns: []string{"uuid", "name", "version"},
				katcTableDefinition: katcTableDefinition{
					SourceType: &katcSourceType{
						name:     sqliteSourceType,
						dataFunc: sqliteData,
					},
					SourcePaths: &[]string{filepath.Join("some", "incorrect", "path")},
					SourceQuery: &testSourceQueryA,
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
			},
			tableNameB: "test_table",
			cfgB: katcTableConfig{
				Columns: []string{"id", "name", "version"},
				katcTableDefinition: katcTableDefinition{
					SourceType: &katcSourceType{
						name:     sqliteSourceType,
						dataFunc: sqliteData,
					},
					SourcePaths: &[]string{filepath.Join("some", "incorrect", "path")},
					SourceQuery: &testSourceQueryA,
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
			},
			expectedEquals: false,
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			testTableA, _ := newKatcTable(tt.tableNameA, tt.cfgA, multislogger.NewNopLogger())
			testTableB, _ := newKatcTable(tt.tableNameB, tt.cfgB, multislogger.NewNopLogger())

			if tt.expectedEquals {
				require.True(t, testTableA.Equals(testTableB))
				require.True(t, testTableB.Equals(testTableA))
			} else {
				require.False(t, testTableA.Equals(testTableB))
				require.False(t, testTableB.Equals(testTableA))
			}
		})
	}
}
