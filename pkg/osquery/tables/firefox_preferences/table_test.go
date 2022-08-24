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

func Test_generateData_HappyPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                    string
		filepath                string
		expectedResultsFilePath string
		errContains             string
	}{
		{
			name:                    "happy path",
			filepath:                "testdata/prefs.js",
			expectedResultsFilePath: "testdata/output.json",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			constraints := make(map[string][]string)
			constraints["path"] = append(constraints["path"], tt.filepath)

			got, err := generateData(tablehelpers.MockQueryContext(constraints), log.NewNopLogger())
			require.NoError(t, err)

			wantBytes, err := os.ReadFile(tt.expectedResultsFilePath)
			require.NoError(t, err)

			var want []map[string]string
			err = json.Unmarshal(wantBytes, &want)
			require.NoError(t, err)

			assert.ElementsMatch(t, want, got)
		})
	}
}
