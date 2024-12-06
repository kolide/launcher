//go:build !windows
// +build !windows

package runtime

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
	typesMocks "github.com/kolide/launcher/ee/agent/types/mocks"
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

	rootDirectory := testRootDirectory(t)

	logBytes, slogger, opts := setUpTestSlogger(rootDirectory)

	k := typesMocks.NewKnapsack(t)
	k.On("RegistrationIDs").Return([]string{types.DefaultRegistrationID})
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("WatchdogEnabled").Return(false)
	k.On("RootDirectory").Return(rootDirectory).Maybe()
	k.On("OsqueryVerbose").Return(true).Maybe()
	k.On("OsqueryFlags").Return([]string{}).Maybe()
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("Slogger").Return(slogger)
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinaryDirectory)
	k.On("LoggingInterval").Return(5 * time.Minute).Maybe()
	k.On("LogMaxBytesPerBatch").Return(0).Maybe()
	k.On("Transport").Return("jsonrpc").Maybe()
	k.On("ReadEnrollSecret").Return("", nil).Maybe()
	setUpMockStores(t, k)

	opts = append(opts, WithStartFunc(func(cmd *exec.Cmd) error {
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
	}))

	runner := New(k, mockServiceClient(), opts...)
	go runner.Run()
	waitHealthy(t, runner, logBytes)

	// ensure that we actually had to wait on the socket
	require.Contains(t, logBytes.String(), "osquery extension socket not created yet")
	waitShutdown(t, runner, logBytes)
}

// TestExtensionSocketPath tests that the launcher can start osqueryd with a custom extension socket path.
// This is only run on non-windows platforms because the extension socket path is semi random on windows.
func TestExtensionSocketPath(t *testing.T) {
	t.Parallel()

	rootDirectory := testRootDirectory(t)

	logBytes, slogger, opts := setUpTestSlogger(rootDirectory)

	k := typesMocks.NewKnapsack(t)
	k.On("RegistrationIDs").Return([]string{types.DefaultRegistrationID})
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("WatchdogEnabled").Return(false)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("Slogger").Return(slogger)
	k.On("RootDirectory").Return(rootDirectory).Maybe()
	k.On("OsqueryVerbose").Return(true).Maybe()
	k.On("OsqueryFlags").Return([]string{}).Maybe()
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinaryDirectory)
	k.On("LoggingInterval").Return(5 * time.Minute).Maybe()
	k.On("LogMaxBytesPerBatch").Return(0).Maybe()
	k.On("Transport").Return("jsonrpc").Maybe()
	k.On("ReadEnrollSecret").Return("", nil).Maybe()
	setUpMockStores(t, k)

	extensionSocketPath := filepath.Join(rootDirectory, "sock")
	opts = append(opts, WithExtensionSocketPath(extensionSocketPath))

	runner := New(k, mockServiceClient(), opts...)
	go runner.Run()

	waitHealthy(t, runner, logBytes)

	// wait for the launcher-provided extension to register
	time.Sleep(2 * time.Second)

	client, err := osquery.NewClient(extensionSocketPath, 5*time.Second, osquery.DefaultWaitTime(1*time.Second), osquery.MaxWaitTime(1*time.Minute))
	require.NoError(t, err)
	defer client.Close()

	resp, err := client.Query("select * from launcher_gc_info")
	require.NoError(t, err)
	assert.Equal(t, int32(0), resp.Status.Code)
	assert.Equal(t, "OK", resp.Status.Message)

	waitShutdown(t, runner, logBytes)
}

// TestRestart tests that the launcher can restart the osqueryd process.
// This test causes time outs on windows, so it is only run on non-windows platforms.
// Should investigate why this is the case.
func TestRestart(t *testing.T) {
	t.Parallel()
	runner, logBytes, teardown := setupOsqueryInstanceForTests(t)
	defer teardown()

	previousStats := runner.instances[types.DefaultRegistrationID].stats

	require.NoError(t, runner.Restart())
	waitHealthy(t, runner, logBytes)

	require.NotEmpty(t, runner.instances[types.DefaultRegistrationID].stats.StartTime, "start time should be set on latest instance stats after restart")
	require.NotEmpty(t, runner.instances[types.DefaultRegistrationID].stats.ConnectTime, "connect time should be set on latest instance stats after restart")

	require.NotEmpty(t, previousStats.ExitTime, "exit time should be set on last instance stats when restarted")
	require.NotEmpty(t, previousStats.Error, "stats instance should have an error on restart")

	previousStats = runner.instances[types.DefaultRegistrationID].stats

	require.NoError(t, runner.Restart())
	waitHealthy(t, runner, logBytes)

	require.NotEmpty(t, runner.instances[types.DefaultRegistrationID].stats.StartTime, "start time should be added to latest instance stats after restart")
	require.NotEmpty(t, runner.instances[types.DefaultRegistrationID].stats.ConnectTime, "connect time should be added to latest instance stats after restart")

	require.NotEmpty(t, previousStats.ExitTime, "exit time should be set on instance stats when restarted")
	require.NotEmpty(t, previousStats.Error, "stats instance should have an error on restart")
}
