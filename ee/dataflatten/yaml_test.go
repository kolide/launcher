package dataflatten

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestYaml(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName string
		fileName     string
		expectedRows []Row
	}{
		{
			testCaseName: "empty",
			fileName:     filepath.Join("testdata", "empty.yaml"),
			expectedRows: []Row{},
		},
		{
			testCaseName: "simple",
			fileName:     filepath.Join("testdata", "simple.yaml"),
			expectedRows: []Row{
				{
					Path:  []string{"name"},
					Value: "GitHub example",
				},
				{
					Path:  []string{"on", "workflow_dispatch", "inputs", "logLevel", "description"},
					Value: "Log level",
				},
				{
					Path:  []string{"on", "workflow_dispatch", "inputs", "logLevel", "required"},
					Value: "true",
				},
				{
					Path:  []string{"on", "workflow_dispatch", "inputs", "logLevel", "default"},
					Value: "warning",
				},
				{
					Path:  []string{"on", "workflow_dispatch", "inputs", "logLevel", "type"},
					Value: "choice",
				},
				{
					Path:  []string{"on", "workflow_dispatch", "inputs", "logLevel", "options", "0"},
					Value: "info",
				},
				{
					Path:  []string{"on", "workflow_dispatch", "inputs", "logLevel", "options", "1"},
					Value: "warning",
				},
				{
					Path:  []string{"on", "workflow_dispatch", "inputs", "logLevel", "options", "2"},
					Value: "debug",
				},
				{
					Path:  []string{"on", "workflow_dispatch", "inputs", "print_tags", "description"},
					Value: "True to print to STDOUT",
				},
				{
					Path:  []string{"on", "workflow_dispatch", "inputs", "print_tags", "required"},
					Value: "true",
				},
				{
					Path:  []string{"on", "workflow_dispatch", "inputs", "print_tags", "type"},
					Value: "boolean",
				},
				{
					Path:  []string{"on", "workflow_dispatch", "inputs", "tags", "description"},
					Value: "Test scenario tags",
				},
				{
					Path:  []string{"on", "workflow_dispatch", "inputs", "tags", "required"},
					Value: "true",
				},
				{
					Path:  []string{"on", "workflow_dispatch", "inputs", "tags", "type"},
					Value: "string",
				},
				{
					Path:  []string{"on", "workflow_dispatch", "inputs", "environment", "description"},
					Value: "Environment to run tests against",
				},
				{
					Path:  []string{"on", "workflow_dispatch", "inputs", "environment", "type"},
					Value: "environment",
				},
				{
					Path:  []string{"on", "workflow_dispatch", "inputs", "environment", "required"},
					Value: "true",
				},
				{
					Path:  []string{"jobs", "print-tag", "runs-on"},
					Value: "ubuntu-latest",
				},
				{
					Path:  []string{"jobs", "print-tag", "if"},
					Value: "${{ inputs.print_tags }}",
				},
				{
					Path:  []string{"jobs", "print-tag", "steps", "0", "name"},
					Value: "Print the input tag to STDOUT",
				},
				{
					Path:  []string{"jobs", "print-tag", "steps", "0", "run"},
					Value: "echo  The tags are ${{ inputs.tags }}",
				},
			},
		},
		{
			testCaseName: "multiple documents in one file",
			fileName:     filepath.Join("testdata", "multiple-docs.yaml"),
			expectedRows: []Row{
				{
					Path:  []string{"document"},
					Value: "1",
				},
				{
					Path:  []string{"document"},
					Value: "2",
				},
			},
		},
	} {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			rawdata, err := os.ReadFile(tt.fileName)
			require.NoError(t, err)

			rows, err := Yaml(rawdata)
			require.NoError(t, err)
			require.Equal(t, len(tt.expectedRows), len(rows))

			// Row ordering is not guaranteed, so confirm that all rows in tt.expectedRows are in `rows`
			// (i.e. no data missing), and then that all entries in `rows` are in tt.expectedRows
			// (i.e. no unexpected data)
			for _, expectedRow := range tt.expectedRows {
				require.Contains(t, rows, expectedRow, "missing expected row")
			}
			for _, row := range rows {
				require.Contains(t, tt.expectedRows, row, "received unexpected row")
			}
		})
	}
}
