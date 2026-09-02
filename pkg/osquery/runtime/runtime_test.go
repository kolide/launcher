package runtime

// these tests have to be run as admin on windows

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/v2/ee/agent/flags/keys"
	"github.com/kolide/launcher/v2/ee/agent/storage"
	storageci "github.com/kolide/launcher/v2/ee/agent/storage/ci"
	"github.com/kolide/launcher/v2/ee/agent/storage/inmemory"
	"github.com/kolide/launcher/v2/ee/agent/types"
	typesMocks "github.com/kolide/launcher/v2/ee/agent/types/mocks"
	"github.com/kolide/launcher/v2/ee/osquerypublisher"
	"github.com/kolide/launcher/v2/pkg/backoff"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	settingsstoremock "github.com/kolide/launcher/v2/pkg/osquery/mocks"
	"github.com/kolide/launcher/v2/pkg/osquery/runtime/history"
	"github.com/kolide/launcher/v2/pkg/osquery/testutil"
	"github.com/kolide/launcher/v2/pkg/service"
	"github.com/kolide/launcher/v2/pkg/threadsafebuffer"
	"github.com/osquery/osquery-go/plugin/distributed"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var testOsqueryBinary string
var osqueryBinaryDownloadErr error

// downloadOnceFunc downloads a real osquery binary for use in tests. This function
// can be called multiple times but will only execute once -- the osquery binary is
// stored at path `testOsqueryBinary` and can be reused by all subsequent tests.
var downloadOnceFunc = sync.OnceFunc(func() {
	testOsqueryBinary, _, osqueryBinaryDownloadErr = testutil.DownloadOsquery("stable")
})

// setupOnceFunc sets up test configuration that should only happen once.
var setupOnceFunc = sync.OnceFunc(func() {
	thrift.ServerConnectivityCheckInterval = 100 * time.Millisecond
})

// requirePermissions checks if the current process has the necessary permissions to run
// tests (elevated permissions on Windows). If not, it skips the test.
func requirePermissions(t *testing.T) {
	if !hasPermissionsToRunTest() {
		t.Skip("these tests must be run as an administrator on windows")
	}
}

func makeTestOsqLogPublisher(t *testing.T, mk *typesMocks.Knapsack) types.OsqueryPublisher {
	// for now, don't enable dual log publication (cutover to new agent-ingester service) for these
	// tests. that logic is tested separately and we can add more logic to test here if needed once
	// we've settled on a cutover plan and desired behaviors
	mk.On("OsqueryPublisherPercentEnabled").Return(0).Maybe()
	mk.On("OsqueryPublisherURL").Return("").Maybe()
	tokenStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.TokenStore.String())
	require.NoError(t, err)
	mk.On("TokenStore").Return(tokenStore).Maybe()
	serverProvidedDataStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.ServerProvidedDataStore.String())
	require.NoError(t, err)
	err = serverProvidedDataStore.Set([]byte("device_id"), []byte("12345"))
	require.NoError(t, err)
	err = serverProvidedDataStore.Set([]byte("organization_id"), []byte("54321"))
	require.NoError(t, err)
	mk.On("ServerProvidedDataStore").Return(serverProvidedDataStore).Maybe()
	slogger := multislogger.NewNopLogger()
	client := &http.Client{}
	t.Cleanup(func() {
		client.CloseIdleConnections()
	})
	return osquerypublisher.NewLogPublisherClient(slogger, mk, client)
}

func TestBadBinaryPath(t *testing.T) {
	t.Parallel()
	requirePermissions(t)
	setupOnceFunc()

	rootDirectory := t.TempDir()

	logBytes, slogger := setUpTestSlogger()

	k := typesMocks.NewKnapsack(t)
	k.On("EnrollmentIDs").Return([]string{types.DefaultEnrollmentID})
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("WatchdogEnabled").Return(false)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("Slogger").Return(slogger)
	k.On("LatestOsquerydPath", mock.Anything).Return("") // bad binary path
	k.On("RootDirectory").Return(rootDirectory).Maybe()
	k.On("OsqueryVerbose").Return(true)
	k.On("OsqueryFlags").Return([]string{})
	k.On("LoggingInterval").Return(5 * time.Minute).Maybe()
	k.On("LogMaxBytesPerBatch").Return(0).Maybe()
	k.On("ReadEnrollSecret").Return("", nil).Maybe()
	k.On("NodeKey", types.DefaultEnrollmentID).Return(ulid.New(), nil).Maybe()
	k.On("EnsureEnrollmentStored", types.DefaultEnrollmentID).Return(nil).Maybe()
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
	setUpMockStores(t, k)
	setupHistory(t, k)
	lpc := makeTestOsqLogPublisher(t, k)
	testServer := setupMockDeviceServer(t)
	k.On("KolideServerURL").Return(testServer).Maybe()
	k.On("InsecureTransportTLS").Return(true).Maybe()

	runner := New(k, lpc, settingsstoremock.NewSettingsStoreWriter(t))
	ensureShutdownOnCleanup(t, runner, logBytes)

	// The runner will repeatedly try to launch the instance, so `Run`
	// won't return an error until we shut it down. Kick off `Run`,
	// wait a while, and confirm we can still shut down.
	go runner.Run()
	time.Sleep(2 * time.Second)
	waitShutdown(t, runner, logBytes)

	// Confirm we tried to launch the instance by examining the logs.
	require.Contains(t, logBytes.String(), "fatal error starting osquery process")

	k.AssertExpectations(t)
}

func TestWithOsqueryFlags(t *testing.T) {
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
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinary)
	k.On("RootDirectory").Return(rootDirectory).Maybe()
	k.On("OsqueryFlags").Return([]string{"verbose=false"})
	k.On("OsqueryVerbose").Return(false)
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
	setupHistory(t, k)
	lpc := makeTestOsqLogPublisher(t, k)
	testServer := setupMockDeviceServer(t)
	k.On("KolideServerURL").Return(testServer).Maybe()
	k.On("InsecureTransportTLS").Return(true).Maybe()

	s := settingsstoremock.NewSettingsStoreWriter(t)
	s.On("WriteSettings").Return(nil).Maybe()

	runner := New(k, lpc, s)
	ensureShutdownOnCleanup(t, runner, logBytes)
	go runner.Run()
	waitHealthy(t, runner, logBytes)
	waitShutdown(t, runner, logBytes)
}

func TestFlagsChanged(t *testing.T) {
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
	k.On("WatchdogMemoryLimitMB").Return(150)
	k.On("WatchdogUtilizationLimitPercent").Return(20)
	k.On("WatchdogDelaySec").Return(120)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("Slogger").Return(slogger)
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinary)
	k.On("RootDirectory").Return(rootDirectory).Maybe()
	k.On("OsqueryFlags").Return([]string{"verbose=false"})
	k.On("OsqueryVerbose").Return(false)
	k.On("LoggingInterval").Return(5 * time.Minute).Maybe()
	k.On("LogMaxBytesPerBatch").Return(0).Maybe()
	k.On("ReadEnrollSecret").Return("", nil).Maybe()
	k.On("NodeKey", types.DefaultEnrollmentID).Return(ulid.New(), nil).Maybe()
	k.On("EnsureEnrollmentStored", types.DefaultEnrollmentID).Return(nil).Maybe()
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
	setupHistory(t, k)
	lpc := makeTestOsqLogPublisher(t, k)
	testServer := setupMockDeviceServer(t)
	k.On("KolideServerURL").Return(testServer).Maybe()
	k.On("InsecureTransportTLS").Return(true).Maybe()

	s := settingsstoremock.NewSettingsStoreWriter(t)
	s.On("WriteSettings").Return(nil).Maybe()

	// set to false initially so osq can start
	k.On("InModernStandby").Return(false)

	// Start the runner
	runner := New(k, lpc, s)
	ensureShutdownOnCleanup(t, runner, logBytes)
	go runner.Run()

	// Wait for the instance to start
	waitHealthy(t, runner, logBytes)

	// Confirm watchdog is disabled -- the instance logs its full args at launch
	require.Contains(t, logBytes.String(), "--disable_watchdog", "instance not set up with watchdog disabled")

	startingRunId := instanceRunId(runner, types.DefaultEnrollmentID)

	// should start off false
	require.False(t, runner.needsRestart.Load(), "runner should not be flagged as needing restart since it just started")

	// change just the InModernStandby flag -- this should not trigger a restart, since no changes need to be applied
	k.On("InModernStandby").Unset()
	k.On("InModernStandby").Return(true)
	runner.FlagsChanged(t.Context(), keys.InModernStandby)
	require.False(t, runner.needsRestart.Load(), "runner should not be marked as needing a restart when only InModernStandby changed")

	// change both the InModernStandby and WatchdogEnabled flags -- this should trigger a restart, but not until InModernStandby is false again
	k.On("WatchdogEnabled").Unset()
	k.On("WatchdogEnabled").Return(true)
	k.On("InModernStandby").Unset()
	k.On("InModernStandby").Return(true)
	runner.FlagsChanged(t.Context(), keys.WatchdogEnabled, keys.InModernStandby)

	require.True(t, runner.needsRestart.Load(), "runner should be marked as needing a restart after WatchdogEnabled changed while in modern standby")

	// no simulate coming out of modern standby -- this should trigger a restart
	logsBeforeRestart := len(logBytes.String())
	k.On("InModernStandby").Unset()
	k.On("InModernStandby").Return(false)
	runner.FlagsChanged(t.Context(), keys.InModernStandby)

	// Wait for the instance to restart, then confirm it's healthy post-restart
	time.Sleep(2 * time.Second)
	waitHealthy(t, runner, logBytes)

	// Now confirm that the instance is new
	require.NotEqual(t, startingRunId, instanceRunId(runner, types.DefaultEnrollmentID), "instance not replaced", logBytes.String())

	require.False(t, runner.needsRestart.Load(), "runner should no longer be marked as needing a restart after restart completed")

	// Confirm osquery watchdog is now enabled -- the instance logs its args at launch
	restartLogs := logBytes.String()[logsBeforeRestart:]
	require.NotContains(t, restartLogs, "--disable_watchdog", "watchdog still disabled after restart")
	require.Contains(t, restartLogs, "--watchdog_memory_limit=150", "watchdog memory limit not set")
	require.Contains(t, restartLogs, "--watchdog_utilization_limit=20", "watchdog CPU limit not set")
	require.Contains(t, restartLogs, "--watchdog_delay=120", "watchdog delay sec not set")

	k.AssertExpectations(t)

	waitShutdown(t, runner, logBytes)
}

func TestPing(t *testing.T) {
	t.Parallel()
	requirePermissions(t)
	downloadOnceFunc()
	require.NoError(t, osqueryBinaryDownloadErr, "could not download osquery, cannot proceed with tests")
	setupOnceFunc()

	// Set up all dependencies
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
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinary)
	k.On("RootDirectory").Return(rootDirectory).Maybe()
	k.On("OsqueryFlags").Return([]string{"verbose=false"})
	k.On("OsqueryVerbose").Return(false)
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
	katcConfigStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.KatcConfigStore.String())
	require.NoError(t, err)
	k.On("KatcConfigStore").Return(katcConfigStore).Maybe()
	k.On("FilewalkResultsStore").Return(inmemory.NewStore()).Maybe()
	k.On("ConfigStore").Return(inmemory.NewStore()).Maybe()
	k.On("EnrollmentStore").Return(inmemory.NewStore()).Maybe()
	k.On("LauncherHistoryStore").Return(inmemory.NewStore()).Maybe()
	k.On("ServerProvidedDataStore").Return(inmemory.NewStore()).Maybe()
	k.On("AgentFlagsStore").Return(inmemory.NewStore()).Maybe()
	k.On("StatusLogsStore").Return(inmemory.NewStore()).Maybe()
	k.On("ResultLogsStore").Return(inmemory.NewStore()).Maybe()
	k.On("BboltDB").Return(storageci.SetupDB(t)).Maybe()
	k.On("WindowsUpdatesCacheStore").Return(inmemory.NewStore()).Maybe()
	setupHistory(t, k)
	s := settingsstoremock.NewSettingsStoreWriter(t)
	s.On("WriteSettings").Return(nil).Maybe()
	k.On("DistributedForwardingInterval").Maybe().Return(60 * time.Second)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything).Maybe().Return()
	k.On("DeregisterChangeObserver", mock.Anything).Maybe().Return()
	k.On("UseCachedDataForScheduledQueries").Return(true).Maybe()
	k.On("ServerReleaseTrackerDataStore").Return(inmemory.NewStore()).Maybe()
	lpc := makeTestOsqLogPublisher(t, k)
	testServer := setupMockDeviceServer(t)
	k.On("KolideServerURL").Return(testServer).Maybe()
	k.On("InsecureTransportTLS").Return(true).Maybe()

	// Start the runner
	runner := New(k, lpc, s)
	ensureShutdownOnCleanup(t, runner, logBytes)
	go runner.Run()

	// Wait for the instance to start
	waitHealthy(t, runner, logBytes)
	startingRunId := instanceRunId(runner, types.DefaultEnrollmentID)

	// Confirm the instance doesn't have the KATC table yet
	testKatcTableName := "katc_test"
	testKatcTableQuery := fmt.Sprintf("SELECT * FROM %s", testKatcTableName)
	_, err = runner.Query(testKatcTableQuery)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no such table")

	// Now, add a KATC config
	tableConfig := map[string]any{
		"columns":      []string{"id"},
		"source_type":  "sqlite",
		"source_query": "",
		"source_paths": []string{},
	}
	tableConfigRaw, err := json.Marshal(tableConfig)
	require.NoError(t, err)
	require.NoError(t, katcConfigStore.Set([]byte(testKatcTableName), tableConfigRaw))
	runner.Ping()

	// Wait for the instance to start its KATC extension manager and confirm the new table is queryable
	err = backoff.WaitFor(func() error {
		if _, err := runner.Query(testKatcTableQuery); err != nil {
			return fmt.Errorf("querying table: %w", err)
		}
		return nil
	}, 10*time.Second, 1*time.Second)
	require.NoError(t, err, "could not query new table", logBytes.String())

	// Confirm that the instance did not restart
	require.Equal(t, startingRunId, instanceRunId(runner, types.DefaultEnrollmentID), "instance restarted, but it should not have")

	// Now, add a new table to our KATC configuration
	secondTestKatcTableName := "katc_test"
	secondTestKatcTableQuery := fmt.Sprintf("SELECT * FROM %s", secondTestKatcTableName)
	secondTableConfig := map[string]any{
		"columns":      []string{"uuid", "name"},
		"source_type":  "sqlite",
		"source_query": "",
		"source_paths": []string{},
	}
	secondTableConfigRaw, err := json.Marshal(secondTableConfig)
	require.NoError(t, err)
	require.NoError(t, katcConfigStore.Set([]byte(secondTestKatcTableName), secondTableConfigRaw))
	runner.Ping()

	// Wait for the instance to restart its KATC extension manager and confirm the second table is queryable
	err = backoff.WaitFor(func() error {
		if _, err := runner.Query(secondTestKatcTableQuery); err != nil {
			return fmt.Errorf("querying table: %w", err)
		}
		return nil
	}, 10*time.Second, 1*time.Second)
	require.NoError(t, err, "could not query new table", logBytes.String())

	// Confirm that the instance did not restart
	require.Equal(t, startingRunId, instanceRunId(runner, types.DefaultEnrollmentID), "instance restarted, but it should not have")

	// Delete both tables from the KATC config
	require.NoError(t, katcConfigStore.Delete([]byte(testKatcTableName), []byte(secondTestKatcTableName)))
	runner.Ping()

	// Confirm we can't query either table anymore
	err = backoff.WaitFor(func() error {
		if _, err := runner.Query(testKatcTableQuery); err == nil {
			return fmt.Errorf("could query %s", testKatcTableName)
		}
		if _, err := runner.Query(secondTestKatcTableQuery); err == nil {
			return fmt.Errorf("could query %s", secondTestKatcTableName)
		}
		return nil
	}, 10*time.Second, 1*time.Second)
	require.NoError(t, err, "able to query deleted tables", logBytes.String())

	// Confirm that the instance did not restart
	require.Equal(t, startingRunId, instanceRunId(runner, types.DefaultEnrollmentID), "instance restarted, but it should not have")

	k.AssertExpectations(t)

	waitShutdown(t, runner, logBytes)
}

func TestSimplePath(t *testing.T) {
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
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinary)
	k.On("RootDirectory").Return(rootDirectory).Maybe()
	k.On("OsqueryFlags").Return([]string{})
	k.On("OsqueryVerbose").Return(true)
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
	setupHistory(t, k)
	testServer := setupMockDeviceServer(t)
	k.On("KolideServerURL").Return(testServer).Maybe()
	k.On("InsecureTransportTLS").Return(true).Maybe()

	s := settingsstoremock.NewSettingsStoreWriter(t)
	s.On("WriteSettings").Return(nil).Maybe()
	lpc := makeTestOsqLogPublisher(t, k)
	runner := New(k, lpc, s)
	ensureShutdownOnCleanup(t, runner, logBytes)
	go runner.Run()

	waitHealthy(t, runner, logBytes)
	waitShutdown(t, runner, logBytes)
}

func TestMultipleInstances(t *testing.T) {
	t.Parallel()
	requirePermissions(t)
	downloadOnceFunc()
	require.NoError(t, osqueryBinaryDownloadErr, "could not download osquery, cannot proceed with tests")
	setupOnceFunc()

	rootDirectory := testRootDirectory(t)

	logBytes, slogger := setUpTestSlogger()

	// Add in an extra instance
	extraEnrollmentId := ulid.New()

	k := typesMocks.NewKnapsack(t)
	k.On("EnrollmentIDs").Return([]string{types.DefaultEnrollmentID, extraEnrollmentId})
	// OsqueryHealthcheckStartupDelay defaults to ten minutes, which is too long for tests.
	// We don't want an extremely short interval, though, because we need to give the osquery instance
	// time to actually start before we begin healthchecking it. So, we wait for at least the
	// amount of time that we give for the socket to appear.
	k.On("OsqueryHealthcheckStartupDelay").Return(socketOpenTimeout).Maybe()
	k.On("WatchdogEnabled").Return(false)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("Slogger").Return(slogger)
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinary)
	k.On("RootDirectory").Return(rootDirectory).Maybe()
	k.On("OsqueryFlags").Return([]string{})
	k.On("OsqueryVerbose").Return(true)
	k.On("LoggingInterval").Return(5 * time.Minute).Maybe()
	k.On("LogMaxBytesPerBatch").Return(0).Maybe()
	k.On("ReadEnrollSecret").Return("", nil).Maybe()
	k.On("NodeKey", types.DefaultEnrollmentID).Return(ulid.New(), nil).Maybe()
	k.On("EnsureEnrollmentStored", types.DefaultEnrollmentID).Return(nil).Maybe()
	k.On("NodeKey", extraEnrollmentId).Return(ulid.New(), nil).Maybe()
	k.On("EnsureEnrollmentStored", extraEnrollmentId).Return(nil).Maybe()
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
	lpc := makeTestOsqLogPublisher(t, k)
	testServer := setupMockDeviceServer(t)
	k.On("KolideServerURL").Return(testServer).Maybe()
	k.On("InsecureTransportTLS").Return(true).Maybe()

	s := settingsstoremock.NewSettingsStoreWriter(t)
	s.On("WriteSettings").Return(nil).Maybe()

	runner := New(k, lpc, s)
	ensureShutdownOnCleanup(t, runner, logBytes)

	// Start the instance
	go runner.Run()
	waitHealthy(t, runner, logBytes)

	// Confirm the default instance was started
	require.NotEmpty(t, instanceRunId(runner, types.DefaultEnrollmentID), "no default instance")

	// Confirm the additional instance was started
	require.NotEmpty(t, instanceRunId(runner, extraEnrollmentId), "no extra instance")
	waitConnected(t, osqHistory, extraEnrollmentId)
	extraInstanceStats, err := osqHistory.LatestInstanceStats(extraEnrollmentId)
	require.NoError(t, err)
	require.Contains(t, extraInstanceStats, "start_time")
	require.Contains(t, extraInstanceStats, "connect_time")
	require.NotEmpty(t, extraInstanceStats["start_time"], "start time should be added to secondary instance stats on start up")
	require.NotEmpty(t, extraInstanceStats["connect_time"], "connect time should be added to secondary instance stats on start up")

	// Confirm instance statuses are reported correctly
	instanceStatuses := runner.InstanceStatuses()
	require.Contains(t, instanceStatuses, types.DefaultEnrollmentID)
	require.Equal(t, instanceStatuses[types.DefaultEnrollmentID], types.InstanceStatusHealthy)
	require.Contains(t, instanceStatuses, extraEnrollmentId)
	require.Equal(t, instanceStatuses[extraEnrollmentId], types.InstanceStatusHealthy)

	waitShutdown(t, runner, logBytes)

	// Confirm both instances exited
	defaultInstanceStats, err := osqHistory.LatestInstanceStats(types.DefaultEnrollmentID)
	require.NoError(t, err)
	require.Contains(t, defaultInstanceStats, "exit_time")
	require.NotEmpty(t, defaultInstanceStats["exit_time"], "exit time should be added to default instance stats on shutdown")

	extraInstanceStats, err = osqHistory.LatestInstanceStats(extraEnrollmentId)
	require.NoError(t, err)
	require.Contains(t, extraInstanceStats, "exit_time")
	require.NotEmpty(t, extraInstanceStats["exit_time"], "exit time should be added to secondary instance stats on shutdown")
}

func TestRunnerHandlesShutdownDuringLaunchWithMultipleInstances(t *testing.T) {
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
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinary)
	k.On("RootDirectory").Return(rootDirectory).Maybe()
	k.On("OsqueryFlags").Return([]string{})
	k.On("OsqueryVerbose").Return(true)
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
	setupHistory(t, k)
	testServer := setupMockDeviceServer(t)
	k.On("KolideServerURL").Return(testServer).Maybe()
	k.On("InsecureTransportTLS").Return(true).Maybe()

	s := settingsstoremock.NewSettingsStoreWriter(t)
	s.On("WriteSettings").Return(nil).Maybe()
	lpc := makeTestOsqLogPublisher(t, k)

	// Park each launch after osqueryd starts until shutdown fires, so the shutdown is
	// guaranteed to arrive mid-launch, with a live osqueryd to clean up
	launchedPids := make(chan int, 2) // buffered for both instances
	var runner *Runner
	runner = New(k, lpc, s, WithStartFunc(func(cmd *exec.Cmd) error {
		if err := cmd.Start(); err != nil {
			return err
		}
		launchedPids <- cmd.Process.Pid
		<-runner.shutdown
		return nil
	}))
	ensureShutdownOnCleanup(t, runner, logBytes)

	// Add in an extra instance
	extraEnrollmentId := ulid.New()
	runner.enrollmentIds = append(runner.enrollmentIds, extraEnrollmentId)

	k.On("NodeKey", extraEnrollmentId).Return(ulid.New(), nil).Maybe()
	k.On("EnsureEnrollmentStored", extraEnrollmentId).Return(nil).Maybe()

	go runner.Run()

	// Shut down once both instances have a live osqueryd parked mid-launch
	firstPid, secondPid := <-launchedPids, <-launchedPids
	waitShutdown(t, runner, logBytes)
	requireProcessGone(t, firstPid)
	requireProcessGone(t, secondPid)
}

func TestMultipleShutdowns(t *testing.T) {
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
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinary)
	k.On("RootDirectory").Return(rootDirectory).Maybe()
	k.On("OsqueryFlags").Return([]string{})
	k.On("OsqueryVerbose").Return(true)
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
	setupHistory(t, k)
	testServer := setupMockDeviceServer(t)
	k.On("KolideServerURL").Return(testServer).Maybe()
	k.On("InsecureTransportTLS").Return(true).Maybe()

	s := settingsstoremock.NewSettingsStoreWriter(t)
	s.On("WriteSettings").Return(nil).Maybe()
	lpc := makeTestOsqLogPublisher(t, k)

	runner := New(k, lpc, s)
	ensureShutdownOnCleanup(t, runner, logBytes)
	go runner.Run()

	waitHealthy(t, runner, logBytes)

	for i := 0; i < 3; i += 1 {
		waitShutdown(t, runner, logBytes)
	}
}

func TestOsqueryDies(t *testing.T) {
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
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinary)
	k.On("RootDirectory").Return(rootDirectory)
	k.On("OsqueryFlags").Return([]string{})
	k.On("OsqueryVerbose").Return(true)
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
	lpc := makeTestOsqLogPublisher(t, k)

	runner := New(k, lpc, s)
	ensureShutdownOnCleanup(t, runner, logBytes)
	go runner.Run()

	waitHealthy(t, runner, logBytes)

	startingRunId := instanceRunId(runner, types.DefaultEnrollmentID)
	require.NotEmpty(t, startingRunId, "no default instance")

	// Simulate the osquery process unexpectedly dying
	require.NoError(t, killProcessGroup(instancePid(t, runner, types.DefaultEnrollmentID)))

	waitHealthyReplacement(t, runner, startingRunId, logBytes)
	allHistory, err := osqHistory.GetHistory()
	require.NoError(t, err, "expected to be able to view osquery history after unexpected shutdown")
	// At least 2 instances: the killed one and a healthy restart. On slow CI runners,
	// there may be additional intermediate restart attempts that failed health checks.
	require.GreaterOrEqual(t, len(allHistory), 2, "expected at least 2 history entries (killed + restarted)")
	firstInstance, lastInstance := allHistory[0], allHistory[len(allHistory)-1]
	// the first instance should have had an error and exit time set
	require.Contains(t, firstInstance, "exit_time")
	require.Contains(t, firstInstance, "errors")
	require.NotEmpty(t, firstInstance["errors"], "error should be added to stats when unexpected shutdown occurs")
	require.NotEmpty(t, firstInstance["exit_time"], "exit time should be added to instance when unexpected shutdown occurs")
	// the last instance is still running -- check that there is no exit time or error set
	require.Contains(t, lastInstance, "exit_time")
	require.Contains(t, lastInstance, "errors")
	require.Empty(t, lastInstance["errors"], "error should not be added to stats for newly created instance")
	require.Empty(t, lastInstance["exit_time"], "exit time should be added to stats for newly created instance")

	waitShutdown(t, runner, logBytes)
}

func TestNotStarted(t *testing.T) {
	t.Parallel()
	requirePermissions(t)
	setupOnceFunc()

	rootDirectory := t.TempDir()

	k := typesMocks.NewKnapsack(t)
	k.On("EnrollmentIDs").Return([]string{types.DefaultEnrollmentID})
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("RootDirectory").Return(rootDirectory).Maybe()
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	k.On("Slogger").Return(multislogger.NewNopLogger())
	setupHistory(t, k)
	lpc := makeTestOsqLogPublisher(t, k)
	runner := New(k, lpc, settingsstoremock.NewSettingsStoreWriter(t))

	assert.Error(t, runner.Healthy())
	runner.Shutdown()
}

// WithStartFunc defines the function that will be used to exeute the osqueryd
// start command. It is useful during testing to simulate osquery start delays or
// osquery instability.
func WithStartFunc(f func(cmd *exec.Cmd) error) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.startFunc = f
	}
}

// TestRestart tests that the launcher can restart the osqueryd process.
func TestRestart(t *testing.T) {
	t.Parallel()
	requirePermissions(t)
	downloadOnceFunc()
	require.NoError(t, osqueryBinaryDownloadErr, "could not download osquery, cannot proceed with tests")
	setupOnceFunc()

	runner, logBytes, osqHistory := setupOsqueryInstanceForTests(t)
	ensureShutdownOnCleanup(t, runner, logBytes)

	firstRunId := instanceRunId(runner, types.DefaultEnrollmentID)
	firstPid := instancePid(t, runner, types.DefaultEnrollmentID)
	waitConnected(t, osqHistory, types.DefaultEnrollmentID)

	require.NoError(t, runner.Restart(t.Context()))
	secondRunId := waitHealthyReplacement(t, runner, firstRunId, logBytes)
	requireProcessGone(t, firstPid)
	secondPid := instancePid(t, runner, types.DefaultEnrollmentID)
	waitConnected(t, osqHistory, types.DefaultEnrollmentID)

	require.NoError(t, runner.Restart(t.Context()))
	waitHealthyReplacement(t, runner, secondRunId, logBytes)
	requireProcessGone(t, secondPid)
	waitConnected(t, osqHistory, types.DefaultEnrollmentID)

	allStats, err := osqHistory.GetHistory()
	require.NoError(t, err, "expected to be able to view osquery history after restarts")
	// we started an instance and then restarted twice, expect 3 entries
	require.Equal(t, 3, len(allStats))

	for idx, stats := range allStats {
		require.Contains(t, stats, "start_time", "expected start time field to be present in stats entry")
		require.NotEmpty(t, stats["start_time"], "expected start time field to be populated in stats entry")
		require.Contains(t, stats, "connect_time", "expected connect time field to be present in stats entry")
		require.NotEmpty(t, stats["connect_time"], "expected connect time field to be populated in stats entry")
		require.Contains(t, stats, "exit_time", "expected exit time field to be present in stats entry")
		require.Contains(t, stats, "errors", "expected errors field to be present in stats entry")

		if idx < 2 { // the latest instance should be healthy still (no exit)
			require.NotEmpty(t, stats["exit_time"], "expected exit time field to be populated in stats entry")
			require.NotEmpty(t, stats["errors"], "expected errors field to be populated in stats entry after restart")
		} else {
			require.Empty(t, stats["exit_time"], "expected exit time field to be empty for latest stats entry")
			require.Empty(t, stats["errors"], "expected errors field to be empty for latest stats entry")
		}
	}

	waitShutdown(t, runner, logBytes)
}

// runner should wait out a slow-starting osqueryd.
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

			runner, logBytes, _ := newTestRunner(t, delayOsqueryd(t, tt.delay))
			ensureShutdownOnCleanup(t, runner, logBytes)
			go runner.Run()

			// nothing can be healthy before the delay elapses, and waiting here keeps
			// waitHealthy's deadline from overlapping the instance's own startup budget
			time.Sleep(tt.delay - 1*time.Second)
			require.Error(t, runner.Healthy(), "healthy before delayed osqueryd could have started")
			time.Sleep(1 * time.Second)

			waitHealthy(t, runner, logBytes)

			waitShutdown(t, runner, logBytes)
		})
	}
}

// sets up an osquery instance and returns it.
func newTestRunner(t *testing.T, opts ...OsqueryInstanceOption) (runner *Runner, logBytes *threadsafebuffer.ThreadSafeBuffer, osqHistory *history.History) {
	rootDirectory := testRootDirectory(t)

	logBytes, slogger := setUpTestSlogger()

	k := typesMocks.NewKnapsack(t)
	k.On("EnrollmentIDs").Return([]string{types.DefaultEnrollmentID})
	// OsqueryHealthcheckStartupDelay defaults to ten minutes, which is too long for tests.
	// We don't want an extremely short interval, though, because we need to give the osquery instance
	// time to actually start before we begin healthchecking it. So, we wait for at least the
	// amount of time that we give for the socket to appear.
	k.On("OsqueryHealthcheckStartupDelay").Return(socketOpenTimeout).Maybe()
	k.On("WatchdogEnabled").Return(true)
	k.On("WatchdogMemoryLimitMB").Return(150)
	k.On("WatchdogUtilizationLimitPercent").Return(20)
	k.On("WatchdogDelaySec").Return(120)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
	k.On("Slogger").Return(slogger)
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinary)
	k.On("RootDirectory").Return(rootDirectory).Maybe()
	k.On("OsqueryFlags").Return([]string{}).Maybe()
	k.On("OsqueryVerbose").Return(true).Maybe()
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
	osqHistory = setupHistory(t, k)
	testServer := setupMockDeviceServer(t)
	k.On("KolideServerURL").Return(testServer).Maybe()
	k.On("InsecureTransportTLS").Return(true).Maybe()

	s := settingsstoremock.NewSettingsStoreWriter(t)
	s.On("WriteSettings").Return(nil).Maybe()
	lpc := makeTestOsqLogPublisher(t, k)

	return New(k, lpc, s, opts...), logBytes, osqHistory
}

// sets up an osquery instance with a running extension to be used in tests.
func setupOsqueryInstanceForTests(t *testing.T) (runner *Runner, logBytes *threadsafebuffer.ThreadSafeBuffer, osqHistory *history.History) {
	runner, logBytes, osqHistory = newTestRunner(t)
	go runner.Run()
	waitHealthy(t, runner, logBytes)

	return runner, logBytes, osqHistory
}

// setUpMockStores creates test stores in the test knapsack
func setUpMockStores(t *testing.T, k *typesMocks.Knapsack) {
	store, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.KatcConfigStore.String())
	require.NoError(t, err)
	k.On("KatcConfigStore").Return(store).Maybe()
	k.On("FilewalkResultsStore").Return(inmemory.NewStore()).Maybe()
	k.On("ConfigStore").Return(inmemory.NewStore()).Maybe()
	k.On("EnrollmentStore").Return(inmemory.NewStore()).Maybe()
	k.On("LauncherHistoryStore").Return(inmemory.NewStore()).Maybe()
	k.On("ServerProvidedDataStore").Return(inmemory.NewStore()).Maybe()
	k.On("AgentFlagsStore").Return(inmemory.NewStore()).Maybe()
	k.On("StatusLogsStore").Return(inmemory.NewStore()).Maybe()
	k.On("ResultLogsStore").Return(inmemory.NewStore()).Maybe()
	k.On("BboltDB").Return(storageci.SetupDB(t)).Maybe()
	k.On("WindowsUpdatesCacheStore").Return(inmemory.NewStore()).Maybe()
	k.On("ServerReleaseTrackerDataStore").Return(inmemory.NewStore()).Maybe()
}

func setupHistory(t *testing.T, k *typesMocks.Knapsack) *history.History {
	store := inmemory.NewStore()
	osqHistory, err := history.InitHistory(store)
	require.NoError(t, err)
	k.On("OsqueryHistory").Return(osqHistory).Maybe()

	return osqHistory
}

// setupMockDeviceServer returns a mock KolideService that returns the minimum possible response
// for all methods.
func setupMockDeviceServer(t *testing.T) string {
	testOptions := map[string]any{
		"distributed_interval": 30,
		"verbose":              true,
		"schedule_epoch":       strconv.Itoa(int(time.Now().Unix())),
	}
	testConfig := map[string]any{
		"options": testOptions,
	}
	testConfigBytes, err := json.Marshal(testConfig)
	require.NoError(t, err)

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		type jsonrpcRequest struct {
			Method string `json:"method"` // there's more but method is all we care about here
		}
		var rDecoded jsonrpcRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&rDecoded))

		var err error
		var respRaw []byte
		switch r.Method {
		case "RequestEnrollment":
			respRaw, err = json.Marshal(&service.EnrollmentResponse{
				NodeKey:            "testnodekey",
				NodeInvalid:        false,
				AgentIngesterToken: "",
			})
			require.NoError(t, err)
		case "RequestConfig":
			respRaw = testConfigBytes
		case "PublishLogs", "PublishResults":
			respRaw = []byte("")
		case "RequestQueries":
			respRaw, err = json.Marshal(&distributed.GetQueriesResult{
				Queries: map[string]string{
					"test-distributed-query": "SELECT * FROM system_info",
				},
			})
			require.NoError(t, err)
		case "CheckHealth":
			respRaw = []byte("1")
		}

		w.Write(respRaw)

	}))

	t.Cleanup(func() {
		testServer.Close()
	})

	return strings.TrimPrefix(testServer.URL, "http://")
}

// setUpTestSlogger sets up a logger that will log to a buffer.
func setUpTestSlogger() (*threadsafebuffer.ThreadSafeBuffer, *slog.Logger) {
	logBytes := &threadsafebuffer.ThreadSafeBuffer{}

	slogger := slog.New(slog.NewTextHandler(logBytes, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}))

	return logBytes, slogger
}

// testRootDirectory returns a temporary directory suitable for use in these tests.
// The default t.TempDir is too long of a path, creating too long of an osquery
// extension socket, on posix systems.
func testRootDirectory(t *testing.T) string {
	var rootDir string

	if runtime.GOOS == "windows" {
		rootDir = t.TempDir()
	} else {
		ulid := ulid.New()
		rootDir = filepath.Join(os.TempDir(), ulid[len(ulid)-4:])
		require.NoError(t, os.Mkdir(rootDir, 0700))
	}

	t.Cleanup(func() {
		// Do a couple retries in case the directory is still in use --
		// Windows is a little slow on this sometimes
		if err := backoff.WaitFor(func() error {
			return os.RemoveAll(rootDir)
		}, 5*time.Second, 500*time.Millisecond); err != nil {
			t.Logf("testRootDirectory RemoveAll cleanup: %v", err)
		}
	})

	return rootDir
}
