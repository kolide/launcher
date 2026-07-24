package dataflattentable

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"testing"

	"github.com/kolide/launcher/v2/ee/dataflatten"
	"github.com/kolide/launcher/v2/ee/tables/tablehelpers"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/stretchr/testify/require"
)

func staticBytes(fn dataflatten.DataFunc) func(table.QueryContext) dataflatten.DataFunc {
	return func(_ table.QueryContext) dataflatten.DataFunc { return fn }
}

func staticFile(fn dataflatten.DataFileFunc) func(table.QueryContext) dataflatten.DataFileFunc {
	return func(_ table.QueryContext) dataflatten.DataFileFunc { return fn }
}

// TestDataFlattenTable_Animals tests the basic generation
// functionality for both plist and json parsing using the mock
// animals data.
func TestDataFlattenTablePlist_Animals(t *testing.T) {
	t.Parallel()

	slogger := multislogger.NewNopLogger()

	// Test plist parsing both the json and xml forms
	testTables := map[string]Table{
		"plist": {slogger: slogger, flattenFileFunc: staticFile(dataflatten.PlistFile), flattenBytesFunc: staticBytes(dataflatten.Plist)},
		"xml":   {slogger: slogger, flattenFileFunc: staticFile(dataflatten.PlistFile), flattenBytesFunc: staticBytes(dataflatten.Plist)},
		"json":  {slogger: slogger, flattenFileFunc: staticFile(dataflatten.JsonFile), flattenBytesFunc: staticBytes(dataflatten.Json)},
		"yaml":  {slogger: slogger, flattenFileFunc: staticFile(dataflatten.YamlFile), flattenBytesFunc: staticBytes(dataflatten.Yaml)},
	}

	var tests = []struct {
		queries  []string
		expected []map[string]string
	}{
		{
			queries: []string{
				"metadata",
			},
			expected: []map[string]string{
				{"fullkey": "metadata/testing", "key": "testing", "parent": "metadata", "value": "true"},
				{"fullkey": "metadata/version", "key": "version", "parent": "metadata", "value": "1.0.1"},
			},
		},
		{
			queries: []string{
				"users/name=>*Aardvark/id",
				"users/name=>*Chipmunk/id",
			},
			expected: []map[string]string{
				{"fullkey": "users/0/id", "key": "id", "parent": "users/0", "value": "1"},
				{"fullkey": "users/2/id", "key": "id", "parent": "users/2", "value": "3"},
			},
		},
	}

	for _, tt := range tests {
		for dataType, tableFunc := range testTables {
			testFile := filepath.Join("testdata", "animals."+dataType)

			// test file path
			mockPathQC := tablehelpers.MockQueryContext(map[string][]string{
				"path":  {testFile},
				"query": tt.queries,
			})

			rows, err := tableFunc.generate(t.Context(), mockPathQC)
			require.NoError(t, err)

			// delete the path, prefilter, and query keys, so we don't need to enumerate them in the test case
			for _, row := range rows {
				delete(row, "path")
				delete(row, "prefilter")
				delete(row, "query")
			}

			// Despite being an array. data is returned unordered. Sort it.
			sort.SliceStable(tt.expected, func(i, j int) bool { return tt.expected[i]["fullkey"] < tt.expected[j]["fullkey"] })
			sort.SliceStable(rows, func(i, j int) bool { return rows[i]["fullkey"] < rows[j]["fullkey"] })

			require.EqualValues(t, tt.expected, rows, "table type %s test", dataType)

			// test bytes path
			raw_data, err := os.ReadFile(testFile)
			require.NoError(t, err)

			mockBytesQC := tablehelpers.MockQueryContext(map[string][]string{
				"raw_data": {string(raw_data)},
				"query":    tt.queries,
			})

			rows, err = tableFunc.generate(t.Context(), mockBytesQC)
			require.NoError(t, err)

			// delete the prefilter, raw_data, and query keys, so we don't need to enumerate them in the test case
			for _, row := range rows {
				delete(row, "prefilter")
				delete(row, "raw_data")
				delete(row, "query")
			}

			// Despite being an array. data is returned unordered. Sort it.
			sort.SliceStable(tt.expected, func(i, j int) bool { return tt.expected[i]["fullkey"] < tt.expected[j]["fullkey"] })
			sort.SliceStable(rows, func(i, j int) bool { return rows[i]["fullkey"] < rows[j]["fullkey"] })

			require.EqualValues(t, tt.expected, rows, "table type %s test", dataType)
		}
	}

}

func TestDataFlattenTablePrefilter_Animals(t *testing.T) {
	t.Parallel()

	slogger := multislogger.NewNopLogger()

	// Test plist parsing both the json and xml forms
	testTables := map[string]Table{
		"plist": {slogger: slogger, flattenFileFunc: staticFile(dataflatten.PlistFile), flattenBytesFunc: staticBytes(dataflatten.Plist)},
		"xml":   {slogger: slogger, flattenFileFunc: staticFile(dataflatten.PlistFile), flattenBytesFunc: staticBytes(dataflatten.Plist)},
		"json":  {slogger: slogger, flattenFileFunc: staticFile(dataflatten.JsonFile), flattenBytesFunc: staticBytes(dataflatten.Json)},
		"yaml":  {slogger: slogger, flattenFileFunc: staticFile(dataflatten.YamlFile), flattenBytesFunc: staticBytes(dataflatten.Yaml)},
	}

	var tests = []struct {
		prefilter string
		query     []string
		expected  []map[string]string
	}{
		{
			prefilter: `type(this) == map && has(this.metadata) ? {"metadata": this.metadata} : {}`,
			expected: []map[string]string{
				{"fullkey": "metadata/testing", "key": "testing", "parent": "metadata", "value": "true"},
				{"fullkey": "metadata/version", "key": "version", "parent": "metadata", "value": "1.0.1"},
			},
		},
		{
			// prefilter handles the filtering; query handles the terminal array rewrite
			prefilter: `type(this) == map && has(this.users) ? {"users": this.users.filter(u, u.name.endsWith("Aardvark") || u.name.endsWith("Chipmunk")).map(u, {"name": u.name, "id": u.id})} : {}`,
			query:     []string{"users/#name/id"},
			expected: []map[string]string{
				{"fullkey": "users/Alex Aardvark/id", "key": "id", "parent": "users/Alex Aardvark", "value": "1"},
				{"fullkey": "users/Cam Chipmunk/id", "key": "id", "parent": "users/Cam Chipmunk", "value": "3"},
			},
		},
	}

	for _, tt := range tests {
		for dataType, tableFunc := range testTables {
			testFile := filepath.Join("testdata", "animals."+dataType)

			// test file path
			mockPathQC := tablehelpers.MockQueryContext(map[string][]string{
				"path":      {testFile},
				"prefilter": {tt.prefilter},
				"query":     tt.query,
			})

			rows, err := tableFunc.generate(t.Context(), mockPathQC)
			require.NoError(t, err)

			// delete the path, prefilter, and query keys, so we don't need to enumerate them in the test case
			for _, row := range rows {
				delete(row, "path")
				delete(row, "prefilter")
				delete(row, "query")
			}

			// Despite being an array. data is returned unordered. Sort it.
			sort.SliceStable(tt.expected, func(i, j int) bool { return tt.expected[i]["fullkey"] < tt.expected[j]["fullkey"] })
			sort.SliceStable(rows, func(i, j int) bool { return rows[i]["fullkey"] < rows[j]["fullkey"] })

			require.EqualValues(t, tt.expected, rows, "table type %s test", dataType)

			// test bytes path
			raw_data, err := os.ReadFile(testFile)
			require.NoError(t, err)

			mockBytesQC := tablehelpers.MockQueryContext(map[string][]string{
				"raw_data":  {string(raw_data)},
				"prefilter": {tt.prefilter},
				"query":     tt.query,
			})

			rows, err = tableFunc.generate(t.Context(), mockBytesQC)
			require.NoError(t, err)

			// delete the prefilter, raw_data, and query keys, so we don't need to enumerate them in the test case
			for _, row := range rows {
				delete(row, "prefilter")
				delete(row, "raw_data")
				delete(row, "query")
			}

			// Despite being an array. data is returned unordered. Sort it.
			sort.SliceStable(tt.expected, func(i, j int) bool { return tt.expected[i]["fullkey"] < tt.expected[j]["fullkey"] })
			sort.SliceStable(rows, func(i, j int) bool { return rows[i]["fullkey"] < rows[j]["fullkey"] })

			require.EqualValues(t, tt.expected, rows, "table type %s test", dataType)
		}
	}
}

func TestDataFlattenTables(t *testing.T) {
	t.Parallel()

	slogger := multislogger.NewNopLogger()

	var tests = []struct {
		testTables   map[string]Table
		testFile     string
		queries      []string
		prefilter    string
		expectedRows int
		expectNoData bool
	}{
		// xml
		{
			testTables:   map[string]Table{"xml": {slogger: slogger, flattenFileFunc: staticFile(dataflatten.XmlFile)}},
			testFile:     path.Join("testdata", "simple.xml"),
			expectedRows: 6,
		},
		{
			testTables:   map[string]Table{"xml": {slogger: slogger, flattenFileFunc: staticFile(dataflatten.XmlFile)}},
			testFile:     path.Join("testdata", "simple.xml"),
			queries:      []string{"simple/Items"},
			expectedRows: 3,
		},
		{
			testTables:   map[string]Table{"xml": {slogger: slogger, flattenFileFunc: staticFile(dataflatten.XmlFile)}},
			testFile:     path.Join("testdata", "simple.xml"),
			prefilter:    `type(this) == map && has(this.simple) ? {"simple": {"Items": this.simple.Items}} : {}`,
			expectedRows: 3,
		},
		{
			testTables:   map[string]Table{"xml": {slogger: slogger, flattenFileFunc: staticFile(dataflatten.XmlFile)}},
			testFile:     path.Join("testdata", "simple.xml"),
			queries:      []string{"this/does/not/exist"},
			expectNoData: true,
		},
		{
			testTables:   map[string]Table{"xml": {slogger: slogger, flattenFileFunc: staticFile(dataflatten.XmlFile)}},
			testFile:     path.Join("testdata", "simple.xml"),
			prefilter:    `type(this) == map && has(this.nonexistent) ? {"nonexistent": this.nonexistent} : {}`,
			expectNoData: true,
		},

		// ini
		{
			testTables:   map[string]Table{"ini": {slogger: slogger, flattenFileFunc: staticFile(dataflatten.IniFile)}},
			testFile:     path.Join("testdata", "secdata.ini"),
			expectedRows: 87,
		},
		{
			testTables:   map[string]Table{"ini": {slogger: slogger, flattenFileFunc: staticFile(dataflatten.IniFile)}},
			testFile:     path.Join("testdata", "secdata.ini"),
			queries:      []string{"Registry Values"},
			expectedRows: 59,
		},
		{
			testTables:   map[string]Table{"ini": {slogger: slogger, flattenFileFunc: staticFile(dataflatten.IniFile)}},
			testFile:     path.Join("testdata", "secdata.ini"),
			prefilter:    `type(this) == map && "Registry Values" in this ? {"Registry Values": this["Registry Values"]} : {}`,
			expectedRows: 59,
		},
		{
			testTables:   map[string]Table{"ini": {slogger: slogger, flattenFileFunc: staticFile(dataflatten.IniFile)}},
			testFile:     path.Join("testdata", "secdata.ini"),
			queries:      []string{"this/does/not/exist"},
			expectNoData: true,
		},
		{
			testTables:   map[string]Table{"ini": {slogger: slogger, flattenFileFunc: staticFile(dataflatten.IniFile)}},
			testFile:     path.Join("testdata", "secdata.ini"),
			prefilter:    `type(this) == map && "this does not exist" in this ? {"this does not exist": this["this does not exist"]} : {}`,
			expectNoData: true,
		},

		// toml
		{
			testTables:   map[string]Table{"toml": {slogger: slogger, flattenFileFunc: staticFile(dataflatten.TomlFile), flattenBytesFunc: staticBytes(dataflatten.Toml)}},
			testFile:     path.Join("testdata", "simple.toml"),
			expectedRows: 5,
		},
		{
			testTables:   map[string]Table{"toml": {slogger: slogger, flattenFileFunc: staticFile(dataflatten.TomlFile), flattenBytesFunc: staticBytes(dataflatten.Toml)}},
			testFile:     path.Join("testdata", "simple.toml"),
			queries:      []string{"metadata"},
			expectedRows: 2,
		},
		{
			testTables:   map[string]Table{"toml": {slogger: slogger, flattenFileFunc: staticFile(dataflatten.TomlFile), flattenBytesFunc: staticBytes(dataflatten.Toml)}},
			testFile:     path.Join("testdata", "simple.toml"),
			prefilter:    `type(this) == map && has(this.metadata) ? {"metadata": this.metadata} : {}`,
			expectedRows: 2,
		},
		{
			testTables:   map[string]Table{"toml": {slogger: slogger, flattenFileFunc: staticFile(dataflatten.TomlFile), flattenBytesFunc: staticBytes(dataflatten.Toml)}},
			testFile:     path.Join("testdata", "simple.toml"),
			queries:      []string{"this/does/not/exist"},
			expectNoData: true,
		},
		{
			testTables:   map[string]Table{"toml": {slogger: slogger, flattenFileFunc: staticFile(dataflatten.TomlFile), flattenBytesFunc: staticBytes(dataflatten.Toml)}},
			testFile:     path.Join("testdata", "simple.toml"),
			prefilter:    `type(this) == map && has(this.nonexistent) ? {"nonexistent": this.nonexistent} : {}`,
			expectNoData: true,
		},
	}

	for testN, tt := range tests {
		for tableName, testTable := range tt.testTables {

			t.Run(fmt.Sprintf("%d/%s", testN, tableName), func(t *testing.T) {
				t.Parallel()

				constraints := map[string][]string{
					"path":  {tt.testFile},
					"query": tt.queries,
				}
				if tt.prefilter != "" {
					constraints["prefilter"] = []string{tt.prefilter}
				}
				mockQC := tablehelpers.MockQueryContext(constraints)

				rows, err := testTable.generate(t.Context(), mockQC)
				require.NoError(t, err)

				if tt.expectNoData {
					require.Len(t, rows, 0)
				} else {
					require.Len(t, rows, tt.expectedRows)
				}

			})
		}
	}

}
