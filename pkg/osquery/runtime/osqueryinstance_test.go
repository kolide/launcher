package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	"github.com/kolide/launcher/ee/agent/storage/inmemory"
	"github.com/kolide/launcher/ee/agent/types"
	typesMocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/log/multislogger"
	settingsstoremock "github.com/kolide/launcher/pkg/osquery/mocks"
	osquerygen "github.com/osquery/osquery-go/gen/osquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCalculateOsqueryPaths(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()

	runId := ulid.New()
	paths, err := calculateOsqueryPaths(rootDir, types.DefaultRegistrationID, runId, osqueryOptions{})

	require.NoError(t, err)

	// ensure that all of our resulting artifact files are in the rootDir that we
	// dictated
	require.Equal(t, rootDir, filepath.Dir(paths.pidfilePath))
	require.Equal(t, rootDir, filepath.Dir(paths.databasePath))

	if runtime.GOOS != "windows" {
		require.Equal(t, rootDir, filepath.Dir(paths.extensionSocketPath))
	} else {
		require.Equal(t, fmt.Sprintf(`\\.\pipe\kolide-osquery-%s`, runId), paths.extensionSocketPath)
	}

	require.Equal(t, rootDir, filepath.Dir(paths.extensionAutoloadPath))
}

func TestCreateOsqueryCommand(t *testing.T) {
	t.Parallel()
	paths := &osqueryFilePaths{
		pidfilePath:           "/foo/bar/osquery-abcd.pid",
		databasePath:          "/foo/bar/osquery.db",
		extensionSocketPath:   "/foo/bar/osquery.sock",
		extensionAutoloadPath: "/foo/bar/osquery.autoload",
	}

	rootDir := t.TempDir()

	k := typesMocks.NewKnapsack(t)
	k.On("WatchdogEnabled").Return(true)
	k.On("WatchdogMemoryLimitMB").Return(150)
	k.On("WatchdogUtilizationLimitPercent").Return(20)
	k.On("WatchdogDelaySec").Return(120)
	k.On("OsqueryVerbose").Return(true)
	k.On("OsqueryFlags").Return([]string{})
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("RootDirectory").Return(rootDir)
	setupHistory(t, k)

	i := newInstance(types.DefaultRegistrationID, k, mockServiceClient(t), settingsstoremock.NewSettingsStoreWriter(t))
	i.paths = paths

	_, err := i.createOsquerydCommand("") // we do not actually exec so don't need to download a real osquery for this test
	require.NoError(t, err)

	k.AssertExpectations(t)
}

func TestCreateOsqueryCommandWithFlags(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	k := typesMocks.NewKnapsack(t)
	k.On("WatchdogEnabled").Return(true)
	k.On("WatchdogMemoryLimitMB").Return(150)
	k.On("WatchdogUtilizationLimitPercent").Return(20)
	k.On("WatchdogDelaySec").Return(120)
	k.On("OsqueryFlags").Return([]string{"verbose=false", "windows_event_channels=foo,bar"})
	k.On("OsqueryVerbose").Return(true)
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("RootDirectory").Return(rootDir)
	setupHistory(t, k)

	i := newInstance(types.DefaultRegistrationID, k, mockServiceClient(t), settingsstoremock.NewSettingsStoreWriter(t))
	i.paths = &osqueryFilePaths{}

	cmd, err := i.createOsquerydCommand("") // we do not actually exec so don't need to download a real osquery for this test
	require.NoError(t, err)

	// count of flags that cannot be overridden with this option
	const nonOverridableFlagsCount = 8

	// Ensure that the provided flags were placed last (so that they can override)
	assert.Equal(
		t,
		[]string{"--verbose=false", "--windows_event_channels=foo,bar"},
		cmd.Args[len(cmd.Args)-2-nonOverridableFlagsCount:len(cmd.Args)-nonOverridableFlagsCount],
	)

	k.AssertExpectations(t)
}

func TestCreateOsqueryCommand_SetsEnabledWatchdogSettingsAppropriately(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	k := typesMocks.NewKnapsack(t)
	k.On("WatchdogEnabled").Return(true)
	k.On("WatchdogMemoryLimitMB").Return(150)
	k.On("WatchdogUtilizationLimitPercent").Return(20)
	k.On("WatchdogDelaySec").Return(120)
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("OsqueryVerbose").Return(true)
	k.On("OsqueryFlags").Return([]string{})
	k.On("RootDirectory").Return(rootDir)
	setupHistory(t, k)

	i := newInstance(types.DefaultRegistrationID, k, mockServiceClient(t), settingsstoremock.NewSettingsStoreWriter(t))
	i.paths = &osqueryFilePaths{}

	cmd, err := i.createOsquerydCommand("") // we do not actually exec so don't need to download a real osquery for this test
	require.NoError(t, err)

	watchdogMemoryLimitMBFound := false
	watchdogUtilizationLimitPercentFound := false
	watchdogDelaySecFound := false
	for _, a := range cmd.Args {
		if strings.Contains(a, "disable_watchdog") {
			t.Error("disable_watchdog flag set")
			t.FailNow()
		}

		if a == "--watchdog_memory_limit=150" {
			watchdogMemoryLimitMBFound = true
			continue
		}

		if a == "--watchdog_utilization_limit=20" {
			watchdogUtilizationLimitPercentFound = true
			continue
		}

		if a == "--watchdog_delay=120" {
			watchdogDelaySecFound = true
			continue
		}
	}

	require.True(t, watchdogMemoryLimitMBFound, "watchdog memory limit not set")
	require.True(t, watchdogUtilizationLimitPercentFound, "watchdog CPU limit not set")
	require.True(t, watchdogDelaySecFound, "watchdog delay sec not set")

	k.AssertExpectations(t)
}

func TestCreateOsqueryCommand_SetsDisabledWatchdogSettingsAppropriately(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	k := typesMocks.NewKnapsack(t)
	k.On("WatchdogEnabled").Return(false)
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("OsqueryVerbose").Return(true)
	k.On("OsqueryFlags").Return([]string{})
	k.On("RootDirectory").Return(rootDir)
	setupHistory(t, k)

	i := newInstance(types.DefaultRegistrationID, k, mockServiceClient(t), settingsstoremock.NewSettingsStoreWriter(t))
	i.paths = &osqueryFilePaths{}

	cmd, err := i.createOsquerydCommand("") // we do not actually exec so don't need to download a real osquery for this test
	require.NoError(t, err)

	disableWatchdogFound := false
	for _, a := range cmd.Args {
		if strings.Contains(a, "watchdog_memory_limit") {
			t.Error("watchdog_memory_limit flag set")
			t.FailNow()
		}

		if strings.Contains(a, "watchdog_utilization_limit") {
			t.Error("watchdog_utilization_limit flag set")
			t.FailNow()
		}

		if strings.Contains(a, "watchdog_delay") {
			t.Error("watchdog_delay flag set")
			t.FailNow()
		}

		if strings.Contains(a, "disable_watchdog") {
			disableWatchdogFound = true
		}
	}

	require.True(t, disableWatchdogFound, "watchdog disabled not set")

	k.AssertExpectations(t)
}

func TestHealthy_DoesNotPassForUnlaunchedInstance(t *testing.T) {
	t.Parallel()

	k := typesMocks.NewKnapsack(t)
	k.On("Slogger").Return(multislogger.NewNopLogger())
	setupHistory(t, k)

	i := newInstance(types.DefaultRegistrationID, k, mockServiceClient(t), settingsstoremock.NewSettingsStoreWriter(t))

	require.Error(t, i.Healthy(), "unlaunched instance should not return healthy status")
}

func TestQuery_ReturnsErrorForUnlaunchedInstance(t *testing.T) {
	t.Parallel()

	k := typesMocks.NewKnapsack(t)
	k.On("Slogger").Return(multislogger.NewNopLogger())
	setupHistory(t, k)

	i := newInstance(types.DefaultRegistrationID, k, mockServiceClient(t), settingsstoremock.NewSettingsStoreWriter(t))

	_, err := i.Query("select * from osquery_info;")
	require.Error(t, err, "should not be able to query unlaunched instance")
}

func Test_healthcheckWithRetries(t *testing.T) {
	t.Parallel()

	k := typesMocks.NewKnapsack(t)
	k.On("Slogger").Return(multislogger.NewNopLogger())
	setupHistory(t, k)
	i := newInstance(types.DefaultRegistrationID, k, mockServiceClient(t), settingsstoremock.NewSettingsStoreWriter(t))

	// No client available, so healthcheck should fail despite retries
	require.Error(t, i.healthcheckWithRetries(t.Context(), 5, 100*time.Millisecond))
}

func TestHealthy(t *testing.T) {
	t.Parallel()
	downloadOnceFunc()

	// Set up instance dependencies
	logBytes, slogger := setUpTestSlogger()
	rootDirectory := testRootDirectory(t)
	k := typesMocks.NewKnapsack(t)
	k.On("WatchdogEnabled").Return(true)
	k.On("WatchdogMemoryLimitMB").Return(150)
	k.On("WatchdogUtilizationLimitPercent").Return(20)
	k.On("WatchdogDelaySec").Return(120)
	k.On("OsqueryFlags").Return([]string{"verbose=true"})
	k.On("OsqueryVerbose").Return(true)
	k.On("Slogger").Return(slogger)
	k.On("RootDirectory").Return(rootDirectory)
	k.On("LoggingInterval").Return(1 * time.Second)
	k.On("LogMaxBytesPerBatch").Return(500)
	k.On("Transport").Return("jsonrpc")
	setUpMockStores(t, k)
	k.On("ReadEnrollSecret").Return("", nil)
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinary)
	k.On("OsqueryHealthcheckStartupDelay").Return(10 * time.Second)
	k.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel).Maybe()
	k.On("RegisterChangeObserver", mock.Anything, keys.PinnedLauncherVersion).Maybe()
	k.On("RegisterChangeObserver", mock.Anything, keys.PinnedOsquerydVersion).Maybe()
	k.On("UpdateChannel").Return("stable").Maybe()
	k.On("PinnedLauncherVersion").Return("").Maybe()
	k.On("PinnedOsquerydVersion").Return("").Maybe()
	k.On("RegistrationIDs").Return([]string{types.DefaultRegistrationID}).Maybe()
	k.On("TableGenerateTimeout").Return(4 * time.Minute).Maybe()
	k.On("RegisterChangeObserver", mock.Anything, keys.TableGenerateTimeout).Return().Maybe()
	k.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{OSVersion: "1", Hostname: "test"}, nil).Maybe()
	k.On("DistributedForwardingInterval").Maybe().Return(60 * time.Second)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything).Maybe().Return()
	k.On("DeregisterChangeObserver", mock.Anything).Maybe().Return()
	k.On("UseCachedDataForScheduledQueries").Return(true).Maybe()
	s := settingsstoremock.NewSettingsStoreWriter(t)
	s.On("WriteSettings").Return(nil)
	setupHistory(t, k)

	// Run the instance
	i := newInstance(types.DefaultRegistrationID, k, mockServiceClient(t), s)
	go i.Launch()

	// Wait for `Healthy` to pass
	require.NoError(t, backoff.WaitFor(func() error {
		return i.Healthy()
	}, 30*time.Second, 1*time.Second), fmt.Sprintf("instance not healthy by %s: instance logs:\n\n%s", time.Now().String(), logBytes.String()))

	// Add a new extension manager server that we can shut down without killing the errgroup
	testAdditionalServerName := "kolide_test_ext"
	require.NoError(t, i.StartOsqueryExtensionManagerServer(testAdditionalServerName, i.extensionManagerClient, nil, true), "adding test server")

	// Confirm we're still in a healthy state
	require.NoError(t, backoff.WaitFor(func() error {
		return i.Healthy()
	}, 10*time.Second, 1*time.Second), fmt.Sprintf("instance not healthy by %s: instance logs:\n\n%s", time.Now().String(), logBytes.String()))

	// Now, shut down our new server
	i.emsLock.Lock()
	require.NoError(t, i.extensionManagerServers[testAdditionalServerName].Shutdown(t.Context()))
	i.emsLock.Unlock()

	// Expect that the healthcheck begins to fail soon
	require.NoError(t, backoff.WaitFor(func() error {
		if err := i.Healthy(); err != nil {
			if strings.Contains(err.Error(), fmt.Sprintf("missing extension %s", testAdditionalServerName)) {
				return nil
			}
			return fmt.Errorf("unexpected healthcheck error: %w", err)
		}

		return errors.New("healthcheck is still passing")
	}, 10*time.Second, 1*time.Second))

	// Now shut down the instance
	i.BeginShutdown()
	shutdownErr := make(chan error)
	go func() {
		shutdownErr <- i.WaitShutdown(t.Context())
	}()

	select {
	case err := <-shutdownErr:
		require.True(t, errors.Is(err, context.Canceled), fmt.Sprintf("unexpected err at %s: %v; instance logs:\n\n%s", time.Now().String(), err, logBytes.String()))
	case <-time.After(1 * time.Minute):
		t.Error("instance did not shut down within timeout", fmt.Sprintf("instance logs: %s", logBytes.String()))
		t.FailNow()
	}

	k.AssertExpectations(t)
}

func TestLaunch(t *testing.T) {
	t.Parallel()
	downloadOnceFunc()

	logBytes, slogger := setUpTestSlogger()
	rootDirectory := testRootDirectory(t)

	k := typesMocks.NewKnapsack(t)
	k.On("WatchdogEnabled").Return(true)
	k.On("WatchdogMemoryLimitMB").Return(150)
	k.On("WatchdogUtilizationLimitPercent").Return(20)
	k.On("WatchdogDelaySec").Return(120)
	k.On("OsqueryFlags").Return([]string{"verbose=true"})
	k.On("OsqueryVerbose").Return(true)
	k.On("Slogger").Return(slogger)
	k.On("RootDirectory").Return(rootDirectory)
	k.On("LoggingInterval").Return(1 * time.Second)
	k.On("LogMaxBytesPerBatch").Return(500)
	k.On("Transport").Return("jsonrpc")
	setUpMockStores(t, k)
	k.On("ReadEnrollSecret").Return("", nil)
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinary)
	k.On("OsqueryHealthcheckStartupDelay").Return(10 * time.Second)
	k.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel).Maybe()
	k.On("RegisterChangeObserver", mock.Anything, keys.PinnedLauncherVersion).Maybe()
	k.On("RegisterChangeObserver", mock.Anything, keys.PinnedOsquerydVersion).Maybe()
	k.On("UpdateChannel").Return("stable").Maybe()
	k.On("PinnedLauncherVersion").Return("").Maybe()
	k.On("PinnedOsquerydVersion").Return("").Maybe()
	k.On("RegistrationIDs").Return([]string{types.DefaultRegistrationID}).Maybe()
	k.On("TableGenerateTimeout").Return(4 * time.Minute).Maybe()
	k.On("RegisterChangeObserver", mock.Anything, keys.TableGenerateTimeout).Return().Maybe()
	k.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{OSVersion: "1", Hostname: "test"}, nil).Maybe()
	k.On("DistributedForwardingInterval").Maybe().Return(60 * time.Second)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything).Maybe().Return()
	k.On("DeregisterChangeObserver", mock.Anything).Maybe().Return()
	k.On("UseCachedDataForScheduledQueries").Return(true).Maybe()

	s := settingsstoremock.NewSettingsStoreWriter(t)
	s.On("WriteSettings").Return(nil).Maybe()
	osqHistory := setupHistory(t, k)

	i := newInstance(types.DefaultRegistrationID, k, mockServiceClient(t), s)
	require.False(t, i.instanceStarted())
	go i.Launch()

	// Wait for the instance to become healthy
	require.NoError(t, backoff.WaitFor(func() error {
		// Instance self-reports as healthy
		if err := i.Healthy(); err != nil {
			return fmt.Errorf("instance not healthy: %w", err)
		}

		// Confirm instance setup is complete
		latestInstanceStats, err := osqHistory.LatestInstanceStats(types.DefaultRegistrationID)
		if err != nil {
			return fmt.Errorf("collecting latest instance stats: %w", err)
		}

		if connectTime, ok := latestInstanceStats["connect_time"]; !ok || connectTime == "" {
			return errors.New("no connect time set yet")
		}

		// Good to go
		return nil
	}, 30*time.Second, 1*time.Second), fmt.Sprintf("instance not healthy by %s: instance logs:\n\n%s", time.Now().String(), logBytes.String()))

	require.True(t, i.instanceStarted())

	// Now wait for full shutdown
	i.BeginShutdown()
	shutdownErr := make(chan error)
	go func() {
		shutdownErr <- i.WaitShutdown(t.Context())
	}()

	select {
	case err := <-shutdownErr:
		require.True(t, errors.Is(err, context.Canceled), fmt.Sprintf("unexpected err at %s: %v; instance logs:\n\n%s", time.Now().String(), err, logBytes.String()))
	case <-time.After(1 * time.Minute):
		t.Error("instance did not shut down within timeout", fmt.Sprintf("instance logs: %s", logBytes.String()))
		t.FailNow()
	}

	k.AssertExpectations(t)
}

func TestReloadKatcExtension(t *testing.T) {
	t.Parallel()
	downloadOnceFunc()

	// Set up all million dependencies
	logBytes, slogger := setUpTestSlogger()
	rootDirectory := testRootDirectory(t)
	k := typesMocks.NewKnapsack(t)
	k.On("WatchdogEnabled").Return(true)
	k.On("WatchdogMemoryLimitMB").Return(150)
	k.On("WatchdogUtilizationLimitPercent").Return(20)
	k.On("WatchdogDelaySec").Return(120)
	k.On("OsqueryFlags").Return([]string{"verbose=true"})
	k.On("OsqueryVerbose").Return(true)
	k.On("Slogger").Return(slogger)
	k.On("RootDirectory").Return(rootDirectory)
	k.On("LoggingInterval").Return(1 * time.Second)
	k.On("LogMaxBytesPerBatch").Return(500)
	k.On("Transport").Return("jsonrpc")
	katcConfigStore, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.KatcConfigStore.String())
	require.NoError(t, err)
	k.On("KatcConfigStore").Return(katcConfigStore).Maybe()
	k.On("ConfigStore").Return(inmemory.NewStore()).Maybe()
	k.On("RegistrationStore").Return(inmemory.NewStore()).Maybe()
	k.On("LauncherHistoryStore").Return(inmemory.NewStore()).Maybe()
	k.On("ServerProvidedDataStore").Return(inmemory.NewStore()).Maybe()
	k.On("AgentFlagsStore").Return(inmemory.NewStore()).Maybe()
	k.On("WindowsUpdatesCacheStore").Return(inmemory.NewStore()).Maybe()
	k.On("StatusLogsStore").Return(inmemory.NewStore()).Maybe()
	k.On("ResultLogsStore").Return(inmemory.NewStore()).Maybe()
	k.On("BboltDB").Return(storageci.SetupDB(t)).Maybe()
	k.On("ReadEnrollSecret").Return("", nil)
	k.On("InModernStandby").Return(false).Maybe()
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinary)
	k.On("OsqueryHealthcheckStartupDelay").Return(10 * time.Second)
	k.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel).Maybe()
	k.On("RegisterChangeObserver", mock.Anything, keys.PinnedLauncherVersion).Maybe()
	k.On("RegisterChangeObserver", mock.Anything, keys.PinnedOsquerydVersion).Maybe()
	k.On("UpdateChannel").Return("stable").Maybe()
	k.On("PinnedLauncherVersion").Return("").Maybe()
	k.On("PinnedOsquerydVersion").Return("").Maybe()
	k.On("RegistrationIDs").Return([]string{types.DefaultRegistrationID}).Maybe()
	k.On("TableGenerateTimeout").Return(4 * time.Minute).Maybe()
	k.On("RegisterChangeObserver", mock.Anything, keys.TableGenerateTimeout).Return().Maybe()
	k.On("GetEnrollmentDetails").Return(types.EnrollmentDetails{OSVersion: "1", Hostname: "test"}, nil).Maybe()
	k.On("DistributedForwardingInterval").Maybe().Return(60 * time.Second)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything).Maybe().Return()
	k.On("DeregisterChangeObserver", mock.Anything).Maybe().Return()
	k.On("UseCachedDataForScheduledQueries").Return(true).Maybe()
	s := settingsstoremock.NewSettingsStoreWriter(t)
	s.On("WriteSettings").Return(nil)
	osqHistory := setupHistory(t, k)

	// Create an instance and launch it
	i := newInstance(types.DefaultRegistrationID, k, mockServiceClient(t), s)
	go i.Launch()

	// Wait for the instance to become healthy
	require.NoError(t, backoff.WaitFor(func() error {
		// Instance self-reports as healthy
		if err := i.Healthy(); err != nil {
			return fmt.Errorf("instance not healthy: %w", err)
		}

		// Confirm instance setup is complete
		latestInstanceStats, err := osqHistory.LatestInstanceStats(types.DefaultRegistrationID)
		if err != nil {
			return fmt.Errorf("collecting latest instance stats: %w", err)
		}

		if connectTime, ok := latestInstanceStats["connect_time"]; !ok || connectTime == "" {
			return errors.New("no connect time set yet")
		}

		// Good to go
		return nil
	}, 30*time.Second, 1*time.Second), fmt.Sprintf("instance not healthy by %s: instance logs:\n\n%s", time.Now().String(), logBytes.String()))

	// We shouldn't have a KATC extension manager server yet
	i.emsLock.Lock()
	require.NotContains(t, i.extensionManagerServers, katcExtensionName)
	i.emsLock.Unlock()

	// Query for a KATC table that doesn't exist yet
	testKatcTableName := "katc_test"
	testKatcTableQuery := fmt.Sprintf("SELECT * FROM %s", testKatcTableName)
	_, err = i.Query(testKatcTableQuery)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no such table")

	// Call ReloadKatcExtension with no changes -- it shouldn't do anything.
	// We still shouldn't have a KATC server or be able to query for the table.
	require.NoError(t, i.ReloadKatcExtension(t.Context()))
	i.emsLock.Lock()
	require.NotContains(t, i.extensionManagerServers, katcExtensionName)
	i.emsLock.Unlock()
	_, err = i.Query(testKatcTableQuery)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no such table")

	// Update KATC configuration to add a new table
	tableConfig := map[string]any{
		"columns":      []string{"id"},
		"source_type":  "sqlite",
		"source_query": "",
		"source_paths": []string{},
	}
	tableConfigRaw, err := json.Marshal(tableConfig)
	require.NoError(t, err)
	require.NoError(t, katcConfigStore.Set([]byte(testKatcTableName), tableConfigRaw))
	require.NoError(t, i.ReloadKatcExtension(t.Context()))

	// We should have an extension manager server for KATC, and it should know about our table
	i.emsLock.Lock()
	require.Contains(t, i.extensionManagerServers, katcExtensionName)
	columnsResponse, err := i.extensionManagerServers[katcExtensionName].Call(t.Context(), "table", testKatcTableName, osquerygen.ExtensionPluginRequest{
		"action": "columns",
	})
	require.NoError(t, err)
	require.Equal(t, 2, len(columnsResponse.Response)) // we expect "id" per the config, plus "path" for every KATC table
	i.emsLock.Unlock()

	// Now, we should be able to query our new table, too.
	// We may need to try a couple times to wait for the server to be fully running.
	err = backoff.WaitFor(func() error {
		if _, err := i.Query(testKatcTableQuery); err != nil {
			return fmt.Errorf("querying table: %w", err)
		}
		return nil
	}, 10*time.Second, 1*time.Second)
	require.NoError(t, err, "could not query new table", logBytes.String())

	// Update KATC configuration to modify existing table
	updatedTableConfig := map[string]any{
		"columns":      []string{"id", "uuid"},
		"source_type":  "sqlite",
		"source_query": "",
		"source_paths": []string{},
	}
	updatedTableConfigRaw, err := json.Marshal(updatedTableConfig)
	require.NoError(t, err)
	require.NoError(t, katcConfigStore.Set([]byte(testKatcTableName), updatedTableConfigRaw))
	require.NoError(t, i.ReloadKatcExtension(t.Context()))

	// We should still have an extension manager server for KATC
	i.emsLock.Lock()
	require.Contains(t, i.extensionManagerServers, katcExtensionName)
	updatedColumnsResponse, err := i.extensionManagerServers[katcExtensionName].Call(t.Context(), "table", testKatcTableName, osquerygen.ExtensionPluginRequest{
		"action": "columns",
	})
	require.NoError(t, err)
	require.Equal(t, 3, len(updatedColumnsResponse.Response)) // we expect "id" and "uuid" per the config, plus "path" for every KATC table
	i.emsLock.Unlock()

	// We should still be able to query our KATC table
	err = backoff.WaitFor(func() error {
		if _, err := i.Query(testKatcTableQuery); err != nil {
			return fmt.Errorf("querying table: %w", err)
		}
		return nil
	}, 10*time.Second, 1*time.Second)
	require.NoError(t, err, "could not query new table", logBytes.String())

	// Delete KATC configuration entirely
	require.NoError(t, katcConfigStore.Delete([]byte(testKatcTableName)))
	require.NoError(t, i.ReloadKatcExtension(t.Context()))

	// We should no longer have an extension manager server for KATC
	i.emsLock.Lock()
	require.NotContains(t, i.extensionManagerServers, katcExtensionName)
	i.emsLock.Unlock()

	// We should no longer be able to query our KATC table
	_, err = i.Query(testKatcTableQuery)
	require.Error(t, err)

	// All done testing -- now wait for full shutdown
	i.BeginShutdown()
	shutdownErr := make(chan error)
	go func() {
		shutdownErr <- i.WaitShutdown(t.Context())
	}()

	select {
	case err := <-shutdownErr:
		require.True(t, errors.Is(err, context.Canceled), fmt.Sprintf("unexpected err at %s: %v; instance logs:\n\n%s", time.Now().String(), err, logBytes.String()))
	case <-time.After(1 * time.Minute):
		t.Error("instance did not shut down within timeout", fmt.Sprintf("instance logs: %s", logBytes.String()))
		t.FailNow()
	}

	k.AssertExpectations(t)
}
