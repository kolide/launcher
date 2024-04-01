//go:build !windows
// +build !windows

package runtime

import (
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	typesMocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/osquery/osquery-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func requirePgidMatch(t *testing.T, pid int) {
	pgid, err := syscall.Getpgid(pid)
	require.NoError(t, err)
	require.Equal(t, pgid, pid)
}

// TestOsquerySlowStart tests that the launcher can handle a slow-starting osqueryd process.
// This this is only enabled on non-Windows platforms because suspending we have not yet
// figured out how to suspend a process on windows via golang.
func TestOsquerySlowStart(t *testing.T) {
	t.Parallel()
	rootDirectory, rmRootDirectory, err := osqueryTempDir()
	require.NoError(t, err)
	defer rmRootDirectory()

	var logBytes threadsafebuffer.ThreadSafeBuffer

	k := typesMocks.NewKnapsack(t)
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("WatchdogEnabled").Return(false)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	slogger := multislogger.New(slog.NewJSONHandler(&logBytes, &slog.HandlerOptions{Level: slog.LevelDebug}))
	k.On("Slogger").Return(slogger.Logger)
	k.On("PinnedOsquerydVersion").Return("")

	runner := New(
		k,
		WithKnapsack(k),
		WithRootDirectory(rootDirectory),
		WithOsquerydBinary(testOsqueryBinaryDirectory),
		WithSlogger(slogger.Logger),
		WithStartFunc(func(cmd *exec.Cmd) error {
			err := cmd.Start()
			if err != nil {
				return fmt.Errorf("unexpected error starting command: %w", err)
			}
			// suspend the process right away
			cmd.Process.Signal(syscall.SIGTSTP)
			go func() {
				// wait a while before resuming the process
				time.Sleep(3 * time.Second)
				cmd.Process.Signal(syscall.SIGCONT)
			}()
			return nil
		}),
	)
	go runner.Run()
	waitHealthy(t, runner)

	// ensure that we actually had to wait on the socket
	require.Contains(t, logBytes.String(), "osquery extension socket not created yet")
	require.NoError(t, runner.Shutdown())
}

// TestExtensionSocketPath tests that the launcher can start osqueryd with a custom extension socket path.
// This is only run on non-windows platforms because the extension socket path is semi random on windows.
func TestExtensionSocketPath(t *testing.T) {
	t.Parallel()

	rootDirectory, rmRootDirectory, err := osqueryTempDir()
	require.NoError(t, err)
	defer rmRootDirectory()

	k := typesMocks.NewKnapsack(t)
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("WatchdogEnabled").Return(false)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("PinnedOsquerydVersion").Return("")

	extensionSocketPath := filepath.Join(rootDirectory, "sock")
	runner := New(
		k,
		WithKnapsack(k),
		WithRootDirectory(rootDirectory),
		WithExtensionSocketPath(extensionSocketPath),
		WithOsquerydBinary(testOsqueryBinaryDirectory),
	)
	go runner.Run()

	waitHealthy(t, runner)

	// wait for the launcher-provided extension to register
	time.Sleep(2 * time.Second)

	client, err := osquery.NewClient(extensionSocketPath, 5*time.Second, osquery.DefaultWaitTime(1*time.Second), osquery.MaxWaitTime(1*time.Minute))
	require.NoError(t, err)
	defer client.Close()

	resp, err := client.Query("select * from launcher_gc_info")
	require.NoError(t, err)
	assert.Equal(t, int32(0), resp.Status.Code)
	assert.Equal(t, "OK", resp.Status.Message)

	require.NoError(t, runner.Shutdown())
}
