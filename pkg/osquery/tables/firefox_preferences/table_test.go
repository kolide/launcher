package firefox_preferences

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_generateData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                    string
		filepaths               []string
		expectedResultsFilePath string
		query                   string
	}{
		{
			name: "no path",
		},
		{
			name:                    "single path",
			filepaths:               []string{"testdata/prefs.js"},
			expectedResultsFilePath: "testdata/output.single_path.json",
		},
		{
			name:                    "single path with query",
			filepaths:               []string{"testdata/prefs.js"},
			expectedResultsFilePath: "testdata/output.single_path_with_query.json",
			query:                   "app.normandy.first_run",
		},
		{
			name:                    "multiple paths",
			filepaths:               []string{"testdata/prefs.js", "testdata/prefs2.js"},
			expectedResultsFilePath: "testdata/output.multiple_paths.json",
		},
		{
			name:                    "file with bad data",
			filepaths:               []string{"testdata/prefs3.js"},
			expectedResultsFilePath: "testdata/output.file_with_bad_data.json",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			constraints := make(map[string][]string)
			constraints["path"] = append(constraints["path"], tt.filepaths...)
			if tt.query != "" {
				constraints["query"] = append(constraints["query"], tt.query)
			}

			got := generateData(tablehelpers.MockQueryContext(constraints), log.NewNopLogger())

			var want []map[string]string

			if len(tt.filepaths) != 0 {
				wantBytes, err := os.ReadFile(tt.expectedResultsFilePath)
				require.NoError(t, err)

				err = json.Unmarshal(wantBytes, &want)
				require.NoError(t, err)
			}

			assert.ElementsMatch(t, want, got)
		})
	}
}
