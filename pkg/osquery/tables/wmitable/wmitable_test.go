// +build windows

package wmitable

import (
	"context"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
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
				continue
			}

			require.NoError(t, err)

			// It's hard to model what we expect to get
			// from a random windows machine. So let's
			// just check for non-empty data.
			assert.GreaterOrEqual(t, len(rows), tt.minRows, "Expected minimum rows")
			for _, row := range rows {
				for column, data := range row {
					assert.NotEmpty(t, column, "column")
					assert.NotEmpty(t, data, "column data")
				}
			}

		})
	}

}
