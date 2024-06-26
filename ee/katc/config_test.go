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
				"kolide_snappy_sqlite_test": fmt.Sprintf(`{
					"type": "sqlite",
					"platform": "%s",
					"columns": ["data"],
					"path": "/some/path/to/db.sqlite",
					"query": "SELECT data FROM object_data JOIN object_store ON (object_data.object_store_id = object_store.id) WHERE object_store.name=\"testtable\";",
					"data_processing_steps": ["snappy"]
				}`, runtime.GOOS),
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
			testCaseName: "invalid table type",
			katcConfig: map[string]string{
				"kolide_snappy_test": fmt.Sprintf(`{
					"type": "unknown_type",
					"platform": "%s",
					"columns": ["data"],
					"path": "/some/path/to/db.sqlite",
					"query": "SELECT data FROM object_data;"
				}`, runtime.GOOS),
			},
			expectedPluginCount: 0,
		},
		{
			testCaseName: "invalid data processing step type",
			katcConfig: map[string]string{
				"kolide_snappy_test": fmt.Sprintf(`{
					"type": "sqlite",
					"platform": "%s",
					"columns": ["data"],
					"path": "/some/path/to/db.sqlite",
					"query": "SELECT data FROM object_data;",
					"data_processing_steps": ["unknown_step"]
				}`, runtime.GOOS),
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
