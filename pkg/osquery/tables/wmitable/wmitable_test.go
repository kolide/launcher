// +build windows

package wmitable

import (
	"context"
	"testing"

	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/stretchr/testify/require"
)

func TestQueries(t *testing.T) {
	t.Parallel()

	wmiTable := Table{}

	var tests = []struct {
		name       string
		class      string
		properties []string
		expected   []map[string]string
		errRequire require.BoolAssertionFunc

		err bool
	}{
		{
			name:       "simple operating system query",
			class:      "Win32_OperatingSystem",
			properties: []string{"name,version"},
			errRequire: require.NoError,
		},
		{
			name:       "queries with non-string types",
			class:      "Win32_OperatingSystem",
			properties: []string{"InstallDate,primary"},
			errRequire: require.NoError,
		},
		{
			name:       "multiple operating system query",
			class:      "Win32_OperatingSystem",
			properties: []string{"name", "version"},
			errRequire: require.NoError,
		},

		{
			name:       "process query",
			class:      "WIN32_process",
			properties: []string{"Caption,CommandLine,CreationDate,Name,Handle,ReadTransferCount"},
			errRequire: require.Error,
		},
		{
			name:       "bad class name",
			class:      "Win32_OperatingSystem;",
			properties: []string{"name,version"},
			errRequire: require.Error,
		},
		{
			name:       "bad properties",
			class:      "Win32_OperatingSystem",
			properties: []string{"name,ver;sion"},
			errRequire: require.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockQC := tablehelpers.MockQueryContext(map[string][]string{
				"class":      tt.class,
				"properties": tt.properties,
			})

			rows, err := wmiTable.generate(context.TODO(), mockQC)
			tt.errRequire(t, err)
		})
	}

}
