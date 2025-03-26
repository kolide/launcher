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

	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/types"
	typesMocks "github.com/kolide/launcher/ee/agent/types/mocks"
	settingsstoremock "github.com/kolide/launcher/pkg/osquery/mocks"
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

	logBytes, slogger := setUpTestSlogger()

	k := typesMocks.NewKnapsack(t)
	k.On("RegistrationIDs").Return([]string{types.DefaultRegistrationID})
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("WatchdogEnabled").Return(false)
	k.On("RootDirectory").Return(rootDirectory).Maybe()
	k.On("OsqueryVerbose").Return(true).Maybe()
	k.On("OsqueryFlags").Return([]string{}).Maybe()
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("Slogger").Return(slogger)
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinary)
	k.On("LoggingInterval").Return(5 * time.Minute).Maybe()
	k.On("LogMaxBytesPerBatch").Return(0).Maybe()
	k.On("Transport").Return("jsonrpc").Maybe()
	k.On("ReadEnrollSecret").Return("", nil).Maybe()
	k.On("InModernStandby").Return(false).Maybe()
	k.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel).Maybe()
	k.On("RegisterChangeObserver", mock.Anything, keys.PinnedLauncherVersion).Maybe()
	k.On("RegisterChangeObserver", mock.Anything, keys.PinnedOsquerydVersion).Maybe()
	k.On("UpdateChannel").Return("stable").Maybe()
	k.On("PinnedLauncherVersion").Return("").Maybe()
	k.On("PinnedOsquerydVersion").Return("").Maybe()
	k.On("TableGenerateTimeout").Return(4 * time.Minute).Maybe()
	k.On("RegisterChangeObserver", mock.Anything, keys.TableGenerateTimeout).Return().Maybe()
	k.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{OSVersion: "1", Hostname: "test"}, nil).Maybe()
	k.On("DistributedForwardingInterval").Maybe().Return(60 * time.Second)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything).Maybe().Return()
	k.On("DeregisterChangeObserver", mock.Anything).Maybe().Return()
	setUpMockStores(t, k)
	osqHistory := setupHistory(t, k)

	s := settingsstoremock.NewSettingsStoreWriter(t)
	s.On("WriteSettings").Return(nil)

	runner := New(k, mockServiceClient(t), s, WithStartFunc(func(cmd *exec.Cmd) error {
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
	ensureShutdownOnCleanup(t, runner, logBytes)
	go runner.Run()
	waitHealthy(t, runner, logBytes, osqHistory)

	// ensure that we actually had to wait on the socket
	require.Contains(t, logBytes.String(), "osquery extension socket not created yet")
	waitShutdown(t, runner, logBytes)
}

// TestExtensionSocketPath tests that the launcher can start osqueryd with a custom extension socket path.
// This is only run on non-windows platforms because the extension socket path is semi random on windows.
func TestExtensionSocketPath(t *testing.T) {
	t.Parallel()

	rootDirectory := testRootDirectory(t)

	logBytes, slogger := setUpTestSlogger()

	k := typesMocks.NewKnapsack(t)
	k.On("RegistrationIDs").Return([]string{types.DefaultRegistrationID})
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("WatchdogEnabled").Return(false)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("Slogger").Return(slogger)
	k.On("RootDirectory").Return(rootDirectory).Maybe()
	k.On("OsqueryVerbose").Return(true).Maybe()
	k.On("OsqueryFlags").Return([]string{}).Maybe()
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinary)
	k.On("LoggingInterval").Return(5 * time.Minute).Maybe()
	k.On("LogMaxBytesPerBatch").Return(0).Maybe()
	k.On("Transport").Return("jsonrpc").Maybe()
	k.On("ReadEnrollSecret").Return("", nil).Maybe()
	k.On("InModernStandby").Return(false).Maybe()
	k.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel).Maybe()
	k.On("RegisterChangeObserver", mock.Anything, keys.PinnedLauncherVersion).Maybe()
	k.On("RegisterChangeObserver", mock.Anything, keys.PinnedOsquerydVersion).Maybe()
	k.On("UpdateChannel").Return("stable").Maybe()
	k.On("PinnedLauncherVersion").Return("").Maybe()
	k.On("PinnedOsquerydVersion").Return("").Maybe()
	k.On("TableGenerateTimeout").Return(4 * time.Minute).Maybe()
	k.On("RegisterChangeObserver", mock.Anything, keys.TableGenerateTimeout).Return().Maybe()
	k.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{OSVersion: "1", Hostname: "test"}, nil).Maybe()
	k.On("DistributedForwardingInterval").Maybe().Return(60 * time.Second)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything).Maybe().Return()
	k.On("DeregisterChangeObserver", mock.Anything).Maybe().Return()
	setUpMockStores(t, k)
	osqHistory := setupHistory(t, k)

	s := settingsstoremock.NewSettingsStoreWriter(t)
	s.On("WriteSettings").Return(nil)

	extensionSocketPath := filepath.Join(rootDirectory, "sock")

	runner := New(k, mockServiceClient(t), s, WithExtensionSocketPath(extensionSocketPath))
	ensureShutdownOnCleanup(t, runner, logBytes)
	go runner.Run()

	waitHealthy(t, runner, logBytes, osqHistory)

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
