//go:build !windows

package runtime

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/v2/ee/agent/flags/keys"
	"github.com/kolide/launcher/v2/ee/agent/types"
	typesMocks "github.com/kolide/launcher/v2/ee/agent/types/mocks"
	settingsstoremock "github.com/kolide/launcher/v2/pkg/osquery/mocks"
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

// delayOsqueryd wraps the configured osqueryd invocation in a sleep script.
func delayOsqueryd(t *testing.T, delay time.Duration) OsqueryInstanceOption {
	script := filepath.Join(t.TempDir(), "delay.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nsleep \"$1\"\nshift\nexec \"$@\"\n"), 0755))

	return WithStartFunc(func(cmd *exec.Cmd) error {
		sh, err := exec.LookPath("sh")
		require.NoError(t, err)

		cmd.Args = append([]string{"sh", script, strconv.Itoa(int(delay.Seconds())), cmd.Path}, cmd.Args[1:]...)
		cmd.Path = sh

		return cmd.Start()
	})
}

// runner should wait out wait out a slow-starting osqueryd.
func TestOsquerySlowStart(t *testing.T) {
	t.Parallel()
	requirePermissions(t)
	downloadOnceFunc()
	require.NoError(t, osqueryBinaryDownloadErr, "could not download osquery, cannot proceed with tests")
	setupOnceFunc()

	for _, tt := range []struct {
		name  string
		delay time.Duration
	}{
		{"slow", osqueryStartupTimeout / 2},
		// longer than we used to wait for the socket alone, so this only passes
		// if the extension client is allowed the full startup budget
		{"slower", osqueryStartupTimeout + socketOpenTimeout/2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runner, logBytes, osqHistory := newTestRunner(t, delayOsqueryd(t, tt.delay))
			ensureShutdownOnCleanup(t, runner, logBytes)
			go runner.Run()

			// nothing can be healthy before the delay elapses, and waiting here keeps
			// waitHealthy's deadline from overlapping the instance's own startup budget
			time.Sleep(tt.delay)
			waitHealthy(t, runner, logBytes, osqHistory)

			waitShutdown(t, runner, logBytes)
		})
	}
}

// TestExtensionSocketPath tests that the launcher can start osqueryd with a custom extension socket path.
// This is only run on non-windows platforms because the extension socket path is semi random on windows.
func TestExtensionSocketPath(t *testing.T) {
	t.Parallel()
	requirePermissions(t)
	downloadOnceFunc()
	require.NoError(t, osqueryBinaryDownloadErr, "could not download osquery, cannot proceed with tests")
	setupOnceFunc()

	rootDirectory := testRootDirectory(t)

	logBytes, slogger := setUpTestSlogger()

	k := typesMocks.NewKnapsack(t)
	k.On("EnrollmentIDs").Return([]string{types.DefaultEnrollmentID})
	// OsqueryHealthcheckStartupDelay defaults to ten minutes, which is too long for tests.
	// We don't want an extremely short interval, though, because we need to give the osquery instance
	// time to actually start before we begin healthchecking it. So, we wait for at least the
	// amount of time that we give for the socket to appear.
	k.On("OsqueryHealthcheckStartupDelay").Return(socketOpenTimeout).Maybe()
	k.On("WatchdogEnabled").Return(false)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("Slogger").Return(slogger)
	k.On("RootDirectory").Return(rootDirectory).Maybe()
	k.On("OsqueryVerbose").Return(true).Maybe()
	k.On("OsqueryFlags").Return([]string{}).Maybe()
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinary)
	k.On("LoggingInterval").Return(5 * time.Minute).Maybe()
	k.On("LogMaxBytesPerBatch").Return(0).Maybe()
	k.On("ReadEnrollSecret").Return("", nil).Maybe()
	k.On("NodeKey", types.DefaultEnrollmentID).Return(ulid.New(), nil).Maybe()
	k.On("EnsureEnrollmentStored", types.DefaultEnrollmentID).Return(nil).Maybe()
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
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything).Maybe().Return()
	k.On("DeregisterChangeObserver", mock.Anything).Maybe().Return()
	k.On("UseCachedDataForScheduledQueries").Return(true).Maybe()
	setUpMockStores(t, k)
	osqHistory := setupHistory(t, k)
	testServer := setupMockDeviceServer(t)
	k.On("KolideServerURL").Return(testServer).Maybe()
	k.On("InsecureTransportTLS").Return(true).Maybe()

	s := settingsstoremock.NewSettingsStoreWriter(t)
	s.On("WriteSettings").Return(nil).Maybe()

	extensionSocketPath := filepath.Join(rootDirectory, "sock")
	lpc := makeTestOsqLogPublisher(t, k)

	runner := New(k, lpc, s, WithExtensionSocketPath(extensionSocketPath))
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
