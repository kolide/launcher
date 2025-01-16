package runtime

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/types"
	typesMocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/log/multislogger"
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

	osquerydPath := testOsqueryBinary

	k := typesMocks.NewKnapsack(t)
	k.On("WatchdogEnabled").Return(true)
	k.On("WatchdogMemoryLimitMB").Return(150)
	k.On("WatchdogUtilizationLimitPercent").Return(20)
	k.On("WatchdogDelaySec").Return(120)
	k.On("OsqueryVerbose").Return(true)
	k.On("OsqueryFlags").Return([]string{})
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("RootDirectory").Return("")

	i := newInstance(types.DefaultRegistrationID, k, mockServiceClient(t))

	_, err := i.createOsquerydCommand(osquerydPath, paths)
	require.NoError(t, err)

	k.AssertExpectations(t)
}

func TestCreateOsqueryCommandWithFlags(t *testing.T) {
	t.Parallel()

	k := typesMocks.NewKnapsack(t)
	k.On("WatchdogEnabled").Return(true)
	k.On("WatchdogMemoryLimitMB").Return(150)
	k.On("WatchdogUtilizationLimitPercent").Return(20)
	k.On("WatchdogDelaySec").Return(120)
	k.On("OsqueryFlags").Return([]string{"verbose=false", "windows_event_channels=foo,bar"})
	k.On("OsqueryVerbose").Return(true)
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("RootDirectory").Return("")

	i := newInstance(types.DefaultRegistrationID, k, mockServiceClient(t))

	cmd, err := i.createOsquerydCommand(
		testOsqueryBinary,
		&osqueryFilePaths{},
	)
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

	k := typesMocks.NewKnapsack(t)
	k.On("WatchdogEnabled").Return(true)
	k.On("WatchdogMemoryLimitMB").Return(150)
	k.On("WatchdogUtilizationLimitPercent").Return(20)
	k.On("WatchdogDelaySec").Return(120)
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("OsqueryVerbose").Return(true)
	k.On("OsqueryFlags").Return([]string{})
	k.On("RootDirectory").Return("")

	i := newInstance(types.DefaultRegistrationID, k, mockServiceClient(t))

	cmd, err := i.createOsquerydCommand(
		testOsqueryBinary,
		&osqueryFilePaths{},
	)
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

	k := typesMocks.NewKnapsack(t)
	k.On("WatchdogEnabled").Return(false)
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("OsqueryVerbose").Return(true)
	k.On("OsqueryFlags").Return([]string{})
	k.On("RootDirectory").Return("")

	i := newInstance(types.DefaultRegistrationID, k, mockServiceClient(t))

	cmd, err := i.createOsquerydCommand(
		testOsqueryBinary,
		&osqueryFilePaths{},
	)
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

	i := newInstance(types.DefaultRegistrationID, k, mockServiceClient(t))

	require.Error(t, i.Healthy(), "unlaunched instance should not return healthy status")
}

func TestQuery_ReturnsErrorForUnlaunchedInstance(t *testing.T) {
	t.Parallel()

	k := typesMocks.NewKnapsack(t)
	k.On("Slogger").Return(multislogger.NewNopLogger())

	i := newInstance(types.DefaultRegistrationID, k, mockServiceClient(t))

	_, err := i.Query("select * from osquery_info;")
	require.Error(t, err, "should not be able to query unlaunched instance")
}

func Test_healthcheckWithRetries(t *testing.T) {
	t.Parallel()

	k := typesMocks.NewKnapsack(t)
	k.On("Slogger").Return(multislogger.NewNopLogger())
	i := newInstance(types.DefaultRegistrationID, k, mockServiceClient(t))

	// No client available, so healthcheck should fail despite retries
	require.Error(t, i.healthcheckWithRetries(context.TODO(), 5, 100*time.Millisecond))
}

func TestLaunch(t *testing.T) {
	t.Parallel()

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

	i := newInstance(types.DefaultRegistrationID, k, mockServiceClient(t))
	go i.Launch()

	// Wait for the instance to become healthy
	require.NoError(t, backoff.WaitFor(func() error {
		// Instance self-reports as healthy
		if err := i.Healthy(); err != nil {
			return fmt.Errorf("instance not healthy: %w", err)
		}

		// Confirm instance setup is complete
		if i.stats == nil || i.stats.ConnectTime == "" {
			return errors.New("no connect time set yet")
		}

		// Good to go
		return nil
	}, 30*time.Second, 1*time.Second), fmt.Sprintf("instance not healthy by %s: instance logs:\n\n%s", time.Now().String(), logBytes.String()))

	// Now wait for full shutdown
	i.BeginShutdown()
	shutdownErr := make(chan error)
	go func() {
		shutdownErr <- i.WaitShutdown()
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
