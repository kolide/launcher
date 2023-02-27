package syft

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/stretchr/testify/require"
)

func TestTable_generate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		filePaths      []string
		expectedResult []map[string]string
		loggedErr      string
	}{
		{
			name:      "happy path",
			filePaths: []string{executablePath(t)},
		},
		{
			name:      "no path",
			filePaths: []string{},
			loggedErr: "no path provided",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var logBytes bytes.Buffer
			table := Table{logger: log.NewLogfmtLogger(&logBytes)}

			constraints := make(map[string][]string)
			constraints["path"] = tt.filePaths
			got, err := table.generate(context.Background(), tablehelpers.MockQueryContext(constraints))
			require.NoError(t, err)

			if tt.loggedErr != "" {
				require.Contains(t, logBytes.String(), tt.loggedErr)
				return
			}

			require.NotEmpty(t, got)
		})
	}
}

func executablePath(t *testing.T) string {
	// get the executable path
	path, err := os.Executable()
	require.NoError(t, err)
	return path
}
