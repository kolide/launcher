package syft

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery/tables/tablehelpers"
	"github.com/stretchr/testify/require"
)

// leaving this in here as an example of the race issue
// func TestRace(t *testing.T) {
// 	src, _ := source.NewFromFile("/opt/homebrew/bin/git")
// 	syft.CatalogPackages(&src, cataloger.DefaultConfig())
// }

func TestTable_generate(t *testing.T) {
	t.Parallel()

	launcherPath := buildLauncher(t)

	tests := []struct {
		name           string
		filePaths      []string
		expectedResult []map[string]string
		loggedErr      string
	}{
		{
			name:      "happy path",
			filePaths: []string{launcherPath},
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

func buildLauncher(t *testing.T) string {
	// To get around the issue mentioned above, build the binary first and set its path as the executable path on the runner.
	executablePath := filepath.Join(t.TempDir(), "syft-test")

	if runtime.GOOS == "windows" {
		executablePath = fmt.Sprintf("%s.exe", executablePath)
	}

	// due to flakey tests we are tracking the time it takes to build and attempting emit a meaningful error if we time out
	timeout := time.Second * 60
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "build", "-o", executablePath, "../../../../cmd/launcher")
	buildStartTime := time.Now()
	out, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("building launcher binary for desktop testing: %w", err)

		if time.Since(buildStartTime) >= timeout {
			err = fmt.Errorf("timeout (%v) met: %w", timeout, err)
		}
	}
	require.NoError(t, err, string(out))

	return executablePath
}
