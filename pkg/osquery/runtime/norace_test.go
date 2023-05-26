//go:build !windows && !race
// +build !windows,!race

package runtime

import (
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOsquerySlowStartNoRace(t *testing.T) {
	t.Parallel()
	rootDirectory, rmRootDirectory, err := osqueryTempDir()
	require.NoError(t, err)
	defer rmRootDirectory()

	runner, err := LaunchInstance(
		WithRootDirectory(rootDirectory),
		WithOsquerydBinary(testOsqueryBinaryDirectory),
		WithStartFunc(func(cmd *exec.Cmd) error {
			// This function simulates a delay in osqueryd starting up in order to test
			// that launcher can handle any delay gracefully.
			// This creates a race condition that the go cause the go test to fail.
			// You can run it with the -race flag, but havent found a clever way to do
			// that without running all the tests over again.
			go func() {
				time.Sleep(2 * time.Second)
				cmd.Start()
			}()
			return nil
		}),
	)
	require.NoError(t, err)
	waitHealthy(t, runner)
	require.NoError(t, runner.Shutdown())
}

// WithStartFunc defines the function that will be used to exeute the osqueryd
// start command. It is useful during testing to simulate osquery start delays or
// osquery instability.
func WithStartFunc(f func(cmd *exec.Cmd) error) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.startFunc = f
	}
}
