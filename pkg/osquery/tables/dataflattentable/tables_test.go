package dataflattentable

import (
	"context"
	"path/filepath"
	"sort"
	"testing"

	"github.com/kolide/launcher/pkg/dataflatten"
	"github.com/kolide/osquery-go/plugin/table"
	"github.com/stretchr/testify/require"
)

// TestDataFlattenTable_Animals tests the basic generation
// functionality for both plist and json parsing using the mock
// animals data.
func TestDataFlattenTable_Animals(t *testing.T) {
	t.Parallel()

	// Test this with both plist and json
	testTables := map[string]Table{
		"plist": Table{dataFunc: dataflatten.PlistFile},
		"xml":   Table{dataFunc: dataflatten.PlistFile},
		"json":  Table{dataFunc: dataflatten.JsonFile},
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
				map[string]string{"fullkey": "metadata/testing", "key": "testing", "parent": "metadata", "value": "true"},
				map[string]string{"fullkey": "metadata/version", "key": "version", "parent": "metadata", "value": "1.0.1"},
			},
		},
		{
			queries: []string{
				"users/name=>*Aardvark/id",
				"users/name=>*Chipmunk/id",
			},
			expected: []map[string]string{
				map[string]string{"fullkey": "users/0/id", "key": "id", "parent": "users/0", "value": "1"},
				map[string]string{"fullkey": "users/2/id", "key": "id", "parent": "users/2", "value": "3"},
			},
		},
	}

	for _, tt := range tests {
		for dataType, tableFunc := range testTables {
			testFile := filepath.Join("testdata", "animals."+dataType)
			rows, err := tableFunc.generate(context.TODO(), mockQueryContext([]string{testFile}, tt.queries))

			require.NoError(t, err)

			// delete the path and query keys, so we don't need to enumerate them in the test case
			for _, row := range rows {
				delete(row, "path")
				delete(row, "query")
			}

			// Despite being an array. data is returned
			// unordered. Sort it.
			sort.SliceStable(tt.expected, func(i, j int) bool { return tt.expected[i]["fullkey"] < tt.expected[j]["fullkey"] })
			sort.SliceStable(rows, func(i, j int) bool { return rows[i]["fullkey"] < rows[j]["fullkey"] })

			require.EqualValues(t, tt.expected, rows, "table type %s test", dataType)
		}
	}

}

func mockQueryContext(paths []string, queries []string) table.QueryContext {
	pathConstraints := make([]table.Constraint, len(paths))
	for i, path := range paths {
		pathConstraints[i].Expression = path
	}
	queryConstraints := make([]table.Constraint, len(queries))
	for i, q := range queries {
		queryConstraints[i].Expression = q
	}

	queryContext := table.QueryContext{
		Constraints: map[string]table.ConstraintList{
			"path":  table.ConstraintList{Constraints: pathConstraints},
			"query": table.ConstraintList{Constraints: queryConstraints},
		},
	}

	return queryContext
}
