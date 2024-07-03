package katc

import (
	_ "embed"
	"fmt"
	"runtime"
	"testing"

	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func TestConstructKATCTables(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName        string
		katcConfig          map[string]string
		expectedPluginCount int
	}{
		{
			testCaseName: "snappy_sqlite",
			katcConfig: map[string]string{
				"tables": fmt.Sprintf(`[
					{
						"name": "kolide_snappy_sqlite_test",
						"source_type": "sqlite",
						"filter": "%s",
						"columns": ["data"],
						"source_paths": ["/some/path/to/db.sqlite"],
						"source_query": "SELECT data FROM object_data JOIN object_store ON (object_data.object_store_id = object_store.id) WHERE object_store.name=\"testtable\";",
						"row_transform_steps": ["snappy"]
					}
				]`, runtime.GOOS),
			},
			expectedPluginCount: 1,
		},
		{
			testCaseName: "multiple plugins",
			katcConfig: map[string]string{
				"tables": fmt.Sprintf(`[
					{
						"name": "test_1",
						"source_type": "sqlite",
						"filter": "%s",
						"columns": ["data"],
						"source_paths": ["/some/path/to/db.sqlite"],
						"source_query": "SELECT data FROM object_data;",
						"row_transform_steps": ["snappy"]
					},
					{
						"name": "test_2",
						"source_type": "sqlite",
						"filter": "%s",
						"columns": ["col1", "col2"],
						"source_paths": ["/some/path/to/a/different/db.sqlite"],
						"source_query": "SELECT col1, col2 FROM some_table;",
						"row_transform_steps": ["camel_to_snake"]
					}
				]`, runtime.GOOS, runtime.GOOS),
			},
			expectedPluginCount: 2,
		},
		{
			testCaseName: "skips invalid tables and returns valid tables",
			katcConfig: map[string]string{
				"tables": fmt.Sprintf(`[
					{
						"name": "not_a_valid_table",
						"source_type": "not a real type",
						"filter": "%s",
						"columns": ["col1", "col2"],
						"source_paths": ["/some/path/to/a/different/db.sqlite"],
						"source_query": "SELECT col1, col2 FROM some_table;",
						"row_transform_steps": ["not a real row transform step"]
					},
					{
						"name": "valid_table",
						"source_type": "sqlite",
						"filter": "%s",
						"columns": ["data"],
						"source_paths": ["/some/path/to/db.sqlite"],
						"source_query": "SELECT data FROM object_data;",
						"row_transform_steps": ["snappy"]
					}
				]`, runtime.GOOS, runtime.GOOS),
			},
			expectedPluginCount: 1,
		},
		{
			testCaseName: "malformed config",
			katcConfig: map[string]string{
				"malformed_test": "this is not a config",
			},
			expectedPluginCount: 0,
		},
		{
			testCaseName: "malformed table",
			katcConfig: map[string]string{
				"tables": "this is not a config",
			},
			expectedPluginCount: 0,
		},
		{
			testCaseName: "invalid table source",
			katcConfig: map[string]string{
				"tables": fmt.Sprintf(`[
					{
						"name": "kolide_snappy_test",
						"source_type": "unknown_source",
						"filter": "%s",
						"columns": ["data"],
						"source_paths": ["/some/path/to/db.sqlite"],
						"source_query": "SELECT data FROM object_data;"
					}
				]`, runtime.GOOS),
			},
			expectedPluginCount: 0,
		},
		{
			testCaseName: "invalid data processing step type",
			katcConfig: map[string]string{
				"tables": fmt.Sprintf(`[
					{
						"name": "kolide_snappy_test",
						"source_type": "sqlite",
						"filter": "%s",
						"columns": ["data"],
						"source_paths": ["/some/path/to/db.sqlite"],
						"source_query": "SELECT data FROM object_data;",
						"row_transform_steps": ["unknown_step"]
					}
				]`, runtime.GOOS),
			},
			expectedPluginCount: 0,
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			plugins := ConstructKATCTables(tt.katcConfig, multislogger.NewNopLogger())
			require.Equal(t, tt.expectedPluginCount, len(plugins), "unexpected number of plugins")
		})
	}
}
