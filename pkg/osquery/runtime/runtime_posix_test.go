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

	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
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

// hasPermissionsToRunTest always return true for non-windows platforms since
// elveated permissions are not required to run the tests
func hasPermissionsToRunTest() bool {
	return true
}

// TestOsquerySlowStart tests that launcher can handle a slow-starting osqueryd process.
// This this is only enabled on non-Windows platforms because we have not yet figured
// out how to suspend and resume a process on Windows via golang.
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
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinaryDirectory)
	store, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.KatcConfigStore.String())
	require.NoError(t, err)
	k.On("KatcConfigStore").Return(store)

	runner := New(
		k,
		WithKnapsack(k),
		WithRootDirectory(rootDirectory),
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
	waitHealthy(t, runner, &logBytes)

	// ensure that we actually had to wait on the socket
	require.Contains(t, logBytes.String(), "osquery extension socket not created yet")
	waitShutdown(t, runner, &logBytes)
}

// TestExtensionSocketPath tests that the launcher can start osqueryd with a custom extension socket path.
// This is only run on non-windows platforms because the extension socket path is semi random on windows.
func TestExtensionSocketPath(t *testing.T) {
	t.Parallel()

	rootDirectory, rmRootDirectory, err := osqueryTempDir()
	require.NoError(t, err)
	defer rmRootDirectory()

	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}))

	k := typesMocks.NewKnapsack(t)
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("WatchdogEnabled").Return(false)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("Slogger").Return(slogger)
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinaryDirectory)
	store, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.KatcConfigStore.String())
	require.NoError(t, err)
	k.On("KatcConfigStore").Return(store)

	extensionSocketPath := filepath.Join(rootDirectory, "sock")
	runner := New(
		k,
		WithKnapsack(k),
		WithRootDirectory(rootDirectory),
		WithExtensionSocketPath(extensionSocketPath),
	)
	go runner.Run()

	waitHealthy(t, runner, &logBytes)

	// wait for the launcher-provided extension to register
	time.Sleep(2 * time.Second)

	client, err := osquery.NewClient(extensionSocketPath, 5*time.Second, osquery.DefaultWaitTime(1*time.Second), osquery.MaxWaitTime(1*time.Minute))
	require.NoError(t, err)
	defer client.Close()

	resp, err := client.Query("select * from launcher_gc_info")
	require.NoError(t, err)
	assert.Equal(t, int32(0), resp.Status.Code)
	assert.Equal(t, "OK", resp.Status.Message)

	waitShutdown(t, runner, &logBytes)
}

// TestRestart tests that the launcher can restart the osqueryd process.
// This test causes time outs on windows, so it is only run on non-windows platforms.
// Should investigate why this is the case.
func TestRestart(t *testing.T) {
	t.Parallel()
	runner, logBytes, teardown := setupOsqueryInstanceForTests(t)
	defer teardown()

	previousStats := runner.instance.stats

	require.NoError(t, runner.Restart())
	waitHealthy(t, runner, logBytes)

	require.NotEmpty(t, runner.instance.stats.StartTime, "start time should be set on latest instance stats after restart")
	require.NotEmpty(t, runner.instance.stats.ConnectTime, "connect time should be set on latest instance stats after restart")

	require.NotEmpty(t, previousStats.ExitTime, "exit time should be set on last instance stats when restarted")
	require.NotEmpty(t, previousStats.Error, "stats instance should have an error on restart")

	previousStats = runner.instance.stats

	require.NoError(t, runner.Restart())
	waitHealthy(t, runner, logBytes)

	require.NotEmpty(t, runner.instance.stats.StartTime, "start time should be added to latest instance stats after restart")
	require.NotEmpty(t, runner.instance.stats.ConnectTime, "connect time should be added to latest instance stats after restart")

	require.NotEmpty(t, previousStats.ExitTime, "exit time should be set on instance stats when restarted")
	require.NotEmpty(t, previousStats.Error, "stats instance should have an error on restart")
}
