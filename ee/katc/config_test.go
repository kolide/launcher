package katc

import (
	_ "embed"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/mock"
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
				"kolide_snappy_sqlite_test": `{
					"source_type": "sqlite",
					"columns": ["data"],
					"source_paths": ["/some/path/to/db.sqlite"],
					"source_query": "SELECT data FROM object_data JOIN object_store ON (object_data.object_store_id = object_store.id) WHERE object_store.name=\"testtable\";",
					"row_transform_steps": ["snappy"],
					"overlays": []
				}`,
			},
			expectedPluginCount: 1,
		},
		{
			testCaseName: "other_sqlite",
			katcConfig: map[string]string{
				"kolide_other_sqlite_test": `{
					"source_type": "sqlite",
					"columns": ["data"],
					"source_paths": ["/some/path/to/db.sqlite"],
					"source_query": "SELECT QUOTE(value) FROM data;",
					"row_transform_steps": ["hex"],
					"overlays": []
				}`,
			},
			expectedPluginCount: 1,
		},
		{
			testCaseName: "indexeddb_leveldb",
			katcConfig: map[string]string{
				"kolide_indexeddb_leveldb_test": `{
					"source_type": "indexeddb_leveldb",
					"columns": ["data"],
					"source_paths": ["/some/path/to/db.indexeddb.leveldb"],
					"source_query": "db.store",
					"row_transform_steps": ["deserialize_chrome"],
					"overlays": []
				}`,
			},
			expectedPluginCount: 1,
		},
		{
			testCaseName: "leveldb",
			katcConfig: map[string]string{
				"kolide_leveldb_test": `{
					"source_type": "leveldb",
					"columns": ["key", "value"],
					"source_paths": ["/some/path/to/db.leveldb"],
					"source_query": "",
					"row_transform_steps": [],
					"overlays": []
				}`,
			},
			expectedPluginCount: 1,
		},
		{
			testCaseName: "overlay",
			katcConfig: map[string]string{
				"kolide_overlay_test": fmt.Sprintf(`{
					"source_type": "indexeddb_leveldb",
					"columns": ["data"],
					"source_paths": ["/some/path/to/db.indexeddb.leveldb"],
					"source_query": "db.store",
					"row_transform_steps": ["deserialize_chrome"],
					"overlays": [
						{
							"filters": {
								"goos": "%s"
							},
							"source_paths": ["/some/different/path/to/db.indexeddb.leveldb"]
						}
					]
				}`, runtime.GOOS),
			},
			expectedPluginCount: 1,
		},
		{
			testCaseName: "multiple plugins",
			katcConfig: map[string]string{
				"test_1": `{
					"source_type": "sqlite",
					"columns": ["data"],
					"source_paths": ["/some/path/to/db.sqlite"],
					"source_query": "SELECT data FROM object_data;",
					"row_transform_steps": ["snappy"],
					"overlays": []
				}`,
				"test_2": `{
					"source_type": "sqlite",
					"columns": ["col1", "col2"],
					"source_paths": ["/some/path/to/a/different/db.sqlite"],
					"source_query": "SELECT col1, col2 FROM some_table;",
					"row_transform_steps": ["camel_to_snake"],
					"overlays": []
				}`,
			},
			expectedPluginCount: 2,
		},
		{
			testCaseName: "skips invalid tables and returns valid tables",
			katcConfig: map[string]string{
				"not_a_valid_table": `{
					"source_type": "not a real type",
						"columns": ["col1", "col2"],
						"source_paths": ["/some/path/to/a/different/db.sqlite"],
						"source_query": "SELECT col1, col2 FROM some_table;",
						"row_transform_steps": ["not a real row transform step"],
						"overlays": []
				}`,
				"valid_table": `{
					"source_type": "sqlite",
						"columns": ["data"],
						"source_paths": ["/some/path/to/db.sqlite"],
						"source_query": "SELECT data FROM object_data;",
						"row_transform_steps": ["snappy"],
						"overlays": []
				}`,
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
			testCaseName: "invalid table source",
			katcConfig: map[string]string{
				"kolide_snappy_test": `{
					"source_type": "unknown_source",
					"columns": ["data"],
					"source_paths": ["/some/path/to/db.sqlite"],
					"source_query": "SELECT data FROM object_data;",
					"overlays": []
				}`,
			},
			expectedPluginCount: 0,
		},
		{
			testCaseName: "invalid data processing step type",
			katcConfig: map[string]string{
				"kolide_snappy_test": `{
					"source_type": "sqlite",
					"columns": ["data"],
					"source_paths": ["/some/path/to/db.sqlite"],
					"source_query": "SELECT data FROM object_data;",
					"row_transform_steps": ["unknown_step"]
				}`,
			},
			expectedPluginCount: 0,
		},
		{
			testCaseName: "invalid leveldb column",
			katcConfig: map[string]string{
				"kolide_leveldb_test": `{
					"source_type": "leveldb",
					"columns": ["key", "config"],
					"source_paths": ["/some/path/to/db.leveldb"],
					"source_query": "",
					"row_transform_steps": [],
					"overlays": []
				}`,
			},
			expectedPluginCount: 0,
		},
	} {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			mockFlags := typesmocks.NewFlags(t)
			mockFlags.On("TableGenerateTimeout").Return(4 * time.Minute).Maybe()
			mockFlags.On("RegisterChangeObserver", mock.Anything, keys.TableGenerateTimeout).Return().Maybe()

			plugins := ConstructKATCTables(tt.katcConfig, mockFlags, multislogger.NewNopLogger())
			require.Equal(t, tt.expectedPluginCount, len(plugins), "unexpected number of plugins")
		})
	}
}
