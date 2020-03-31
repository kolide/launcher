// +build windows

package wmitable

import (
	"context"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueries(t *testing.T) {
	t.Parallel()

	wmiTable := Table{logger: log.NewNopLogger()}

	var tests = []struct {
		name       string
		class      string
		properties []string
		minRows    int
		err        bool
	}{
		{
			name:       "simple operating system query",
			class:      "Win32_OperatingSystem",
			properties: []string{"name,version"},
			minRows:    1,
		},
		{
			name:       "queries with non-string types",
			class:      "Win32_OperatingSystem",
			properties: []string{"InstallDate,primary"},
			minRows:    1,
		},
		{
			name:       "multiple operating system query",
			class:      "Win32_OperatingSystem",
			properties: []string{"name", "version"},
			minRows:    1,
		},

		{
			name:       "process query",
			class:      "WIN32_process",
			properties: []string{"Caption,CommandLine,CreationDate,Name,Handle,ReadTransferCount"},
			minRows:    10,
		},
		{
			name:       "bad class name",
			class:      "Win32_OperatingSystem;",
			properties: []string{"name,version"},
			err:        true,
		},
		{
			name:       "bad properties",
			class:      "Win32_OperatingSystem",
			properties: []string{"name,ver;sion"},
			err:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockQC := tablehelpers.MockQueryContext(map[string][]string{
				"class":      []string{tt.class},
				"properties": tt.properties,
			})

			rows, err := wmiTable.generate(context.TODO(), mockQC)

			if tt.err {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// It's hard to model what we expect to get
			// from a random windows machine. So let's
			// just check for non-empty data.
			assert.GreaterOrEqual(t, len(rows), tt.minRows, "Expected minimum rows")
			for _, row := range rows {
				// this has gone through dataflatten. Test for various expected results
				require.Contains(t, row, "class", "class column")
				require.Equal(t, tt.class, row["class"], "class name is equal")

				for _, columnName := range []string{"fullkey", "parent", "key", "value"} {
					require.Contains(t, row, columnName, "%s column", columnName)
					assert.NotEmpty(t, tt.class, row[columnName], "%s column not empty", columnName)
				}
			}
		})
	}
}
