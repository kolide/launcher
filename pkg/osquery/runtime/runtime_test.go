package runtime

// these tests have to be run as admin on windows

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/kolide/kit/fsutil"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/agent"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	"github.com/kolide/launcher/ee/agent/storage/inmemory"
	"github.com/kolide/launcher/ee/agent/types"
	typesMocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/osquery/runtime/history"
	"github.com/kolide/launcher/pkg/packaging"
	"github.com/kolide/launcher/pkg/service"
	servicemock "github.com/kolide/launcher/pkg/service/mock"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/osquery/osquery-go/plugin/distributed"
	"github.com/osquery/osquery-go/plugin/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var testOsqueryBinaryDirectory string

// TestMain overrides the default test main function. This allows us to share setup/teardown.
func TestMain(m *testing.M) {
	if !hasPermissionsToRunTest() {
		fmt.Println("these tests must be run as an administrator on windows")
		return
	}

	binDirectory, err := agent.MkdirTemp("")
	if err != nil {
		fmt.Println("Failed to make temp dir for test binaries")
		os.Exit(1) //nolint:forbidigo // Fine to use os.Exit in tests
	}
	defer os.Remove(binDirectory)

	s, err := storageci.NewStore(nil, multislogger.NewNopLogger(), storage.OsqueryHistoryInstanceStore.String())
	if err != nil {
		fmt.Println("Failed to make new store")
		os.Exit(1) //nolint:forbidigo // Fine to use os.Exit in tests
	}
	if err := history.InitHistory(s); err != nil {
		fmt.Println("Failed to init history")
		os.Exit(1) //nolint:forbidigo // Fine to use os.Exit in tests
	}

	testOsqueryBinaryDirectory = filepath.Join(binDirectory, "osqueryd")

	thrift.ServerConnectivityCheckInterval = 100 * time.Millisecond

	if err := downloadOsqueryInBinDir(binDirectory); err != nil {
		fmt.Printf("Failed to download osquery: %v\n", err)
		os.Exit(1) //nolint:forbidigo // Fine to use os.Exit in tests
	}

	// Run the tests!
	retCode := m.Run()
	os.Exit(retCode) //nolint:forbidigo // Fine to use os.Exit in tests
}

// downloadOsqueryInBinDir downloads osqueryd. This allows the test
// suite to run on hosts lacking osqueryd. We could consider moving this into a deps step.
func downloadOsqueryInBinDir(binDirectory string) error {
	target := packaging.Target{}
	if err := target.PlatformFromString(runtime.GOOS); err != nil {
		return fmt.Errorf("Error parsing platform: %s: %w", runtime.GOOS, err)
	}
	target.Arch = packaging.ArchFlavor(runtime.GOARCH)
	if runtime.GOOS == "darwin" {
		target.Arch = packaging.Universal
	}

	outputFile := filepath.Join(binDirectory, "osqueryd")
	if runtime.GOOS == "windows" {
		outputFile += ".exe"
	}

	cacheDir := "/tmp"
	if runtime.GOOS == "windows" {
		cacheDir = os.Getenv("TEMP")
	}

	path, err := packaging.FetchBinary(context.TODO(), cacheDir, "osqueryd", target.PlatformBinaryName("osqueryd"), "stable", target)
	if err != nil {
		return fmt.Errorf("An error occurred fetching the osqueryd binary: %w", err)
	}

	if err := fsutil.CopyFile(path, outputFile); err != nil {
		return fmt.Errorf("Couldn't copy file to %s: %w", outputFile, err)
	}

	return nil
}

func TestBadBinaryPath(t *testing.T) {
	t.Parallel()
	rootDirectory := t.TempDir()

	logBytes, slogger := setUpTestSlogger()

	k := typesMocks.NewKnapsack(t)
	k.On("RegistrationIDs").Return([]string{types.DefaultRegistrationID})
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("WatchdogEnabled").Return(false)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("Slogger").Return(slogger)
	k.On("LatestOsquerydPath", mock.Anything).Return("") // bad binary path
	k.On("RootDirectory").Return(rootDirectory).Maybe()
	k.On("OsqueryVerbose").Return(true)
	k.On("OsqueryFlags").Return([]string{})
	k.On("LoggingInterval").Return(5 * time.Minute).Maybe()
	k.On("LogMaxBytesPerBatch").Return(0).Maybe()
	k.On("Transport").Return("jsonrpc").Maybe()
	k.On("ReadEnrollSecret").Return("", nil).Maybe()
	setUpMockStores(t, k)

	runner := New(k, mockServiceClient())

	// The runner will repeatedly try to launch the instance, so `Run`
	// won't return an error until we shut it down. Kick off `Run`,
	// wait a while, and confirm we can still shut down.
	go runner.Run()
	time.Sleep(2 * time.Second)
	waitShutdown(t, runner, logBytes)

	// Confirm we tried to launch the instance by examining the logs.
	require.Contains(t, logBytes.String(), "could not launch instance, will retry after delay")

	k.AssertExpectations(t)
}

func TestWithOsqueryFlags(t *testing.T) {
	t.Parallel()
	rootDirectory := testRootDirectory(t)

	logBytes, slogger := setUpTestSlogger()

	k := typesMocks.NewKnapsack(t)
	k.On("RegistrationIDs").Return([]string{types.DefaultRegistrationID})
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("WatchdogEnabled").Return(false)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("Slogger").Return(slogger)
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinaryDirectory)
	k.On("RootDirectory").Return(rootDirectory).Maybe()
	k.On("OsqueryFlags").Return([]string{"verbose=false"})
	k.On("OsqueryVerbose").Return(false)
	k.On("LoggingInterval").Return(5 * time.Minute).Maybe()
	k.On("LogMaxBytesPerBatch").Return(0).Maybe()
	k.On("Transport").Return("jsonrpc").Maybe()
	k.On("ReadEnrollSecret").Return("", nil).Maybe()
	setUpMockStores(t, k)

	runner := New(k, mockServiceClient())
	go runner.Run()
	waitHealthy(t, runner, logBytes)
	waitShutdown(t, runner, logBytes)
}

func TestFlagsChanged(t *testing.T) {
	t.Parallel()

	rootDirectory := testRootDirectory(t)

	logBytes, slogger := setUpTestSlogger()

	k := typesMocks.NewKnapsack(t)
	k.On("RegistrationIDs").Return([]string{types.DefaultRegistrationID})
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	// First, it should return false, then on the next call, it should return true
	k.On("WatchdogEnabled").Return(false).Once()
	k.On("WatchdogEnabled").Return(true).Once()
	k.On("WatchdogMemoryLimitMB").Return(150)
	k.On("WatchdogUtilizationLimitPercent").Return(20)
	k.On("WatchdogDelaySec").Return(120)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("Slogger").Return(slogger)
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinaryDirectory)
	k.On("RootDirectory").Return(rootDirectory).Maybe()
	k.On("OsqueryFlags").Return([]string{"verbose=false"})
	k.On("OsqueryVerbose").Return(false)
	k.On("LoggingInterval").Return(5 * time.Minute).Maybe()
	k.On("LogMaxBytesPerBatch").Return(0).Maybe()
	k.On("Transport").Return("jsonrpc").Maybe()
	k.On("ReadEnrollSecret").Return("", nil).Maybe()
	setUpMockStores(t, k)

	// Start the runner
	runner := New(k, mockServiceClient())
	go runner.Run()

	// Wait for the instance to start
	time.Sleep(2 * time.Second)
	waitHealthy(t, runner, logBytes)

	// Confirm watchdog is disabled
	watchdogDisabled := false
	for _, a := range runner.instances[types.DefaultRegistrationID].cmd.Args {
		if strings.Contains(a, "disable_watchdog") {
			watchdogDisabled = true
			break
		}
	}
	require.True(t, watchdogDisabled, "instance not set up with watchdog disabled")

	startingInstance := runner.instances[types.DefaultRegistrationID]

	runner.FlagsChanged(keys.WatchdogEnabled)

	// Wait for the instance to restart
	time.Sleep(2 * time.Second)
	waitHealthy(t, runner, logBytes)

	// Now confirm that the instance is new
	require.NotEqual(t, startingInstance, runner.instances[types.DefaultRegistrationID], "instance not replaced")

	// Confirm osquery watchdog is now enabled
	watchdogMemoryLimitMBFound := false
	watchdogUtilizationLimitPercentFound := false
	watchdogDelaySecFound := false
	for _, a := range runner.instances[types.DefaultRegistrationID].cmd.Args {
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

	waitShutdown(t, runner, logBytes)
}

func waitShutdown(t *testing.T, runner *Runner, logBytes *threadsafebuffer.ThreadSafeBuffer) {
	// We don't want to retry shutdowns because subsequent shutdown calls don't do anything --
	// they return nil immediately, which would give `backoff` the impression that shutdown has
	// completed when it hasn't.
	// Instead, call `Shutdown` once, wait for our timeout (1 minute), and report failure if
	// `Shutdown` has not returned.
	shutdownErr := make(chan error)
	go func() {
		shutdownErr <- runner.Shutdown()
	}()

	select {
	case err := <-shutdownErr:
		require.NoError(t, err, fmt.Sprintf("runner logs:\n\n%s", logBytes.String()))
	case <-time.After(1 * time.Minute):
		t.Error("runner did not shut down within timeout", fmt.Sprintf("runner logs: %s", logBytes.String()))
		t.FailNow()
	}
}

// waitHealthy expects the instance to be healthy within 30 seconds, or else
// fatals the test.
func waitHealthy(t *testing.T, runner *Runner, logBytes *threadsafebuffer.ThreadSafeBuffer) {
	require.NoError(t, backoff.WaitFor(func() error {
		// Instance self-reports as healthy
		if err := runner.Healthy(); err != nil {
			return fmt.Errorf("instance not healthy: %w", err)
		}

		// Confirm osquery instance setup is complete
		if runner.instances[types.DefaultRegistrationID] == nil {
			return errors.New("default instance does not exist yet")
		}
		if runner.instances[types.DefaultRegistrationID].stats == nil || runner.instances[types.DefaultRegistrationID].stats.ConnectTime == "" {
			return errors.New("no connect time set yet")
		}

		// Good to go
		return nil
	}, 30*time.Second, 1*time.Second), fmt.Sprintf("instance not healthy by %s: runner logs:\n\n%s", time.Now().String(), logBytes.String()))

	// Give the instance just a little bit of buffer before we proceed
	time.Sleep(2 * time.Second)
}

func TestSimplePath(t *testing.T) {
	t.Parallel()
	rootDirectory := testRootDirectory(t)

	logBytes, slogger := setUpTestSlogger()

	k := typesMocks.NewKnapsack(t)
	k.On("RegistrationIDs").Return([]string{types.DefaultRegistrationID})
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("WatchdogEnabled").Return(false)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("Slogger").Return(slogger)
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinaryDirectory)
	k.On("RootDirectory").Return(rootDirectory).Maybe()
	k.On("OsqueryFlags").Return([]string{})
	k.On("OsqueryVerbose").Return(true)
	k.On("LoggingInterval").Return(5 * time.Minute).Maybe()
	k.On("LogMaxBytesPerBatch").Return(0).Maybe()
	k.On("Transport").Return("jsonrpc").Maybe()
	k.On("ReadEnrollSecret").Return("", nil).Maybe()
	setUpMockStores(t, k)

	runner := New(k, mockServiceClient())
	go runner.Run()

	waitHealthy(t, runner, logBytes)

	require.NotEmpty(t, runner.instances[types.DefaultRegistrationID].stats.StartTime, "start time should be added to instance stats on start up")
	require.NotEmpty(t, runner.instances[types.DefaultRegistrationID].stats.ConnectTime, "connect time should be added to instance stats on start up")

	waitShutdown(t, runner, logBytes)
}

func TestMultipleInstances(t *testing.T) {
	t.Parallel()
	rootDirectory := testRootDirectory(t)

	logBytes, slogger := setUpTestSlogger()

	// Add in an extra instance
	extraRegistrationId := ulid.New()

	k := typesMocks.NewKnapsack(t)
	k.On("RegistrationIDs").Return([]string{types.DefaultRegistrationID, extraRegistrationId})
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("WatchdogEnabled").Return(false)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("Slogger").Return(slogger)
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinaryDirectory)
	k.On("RootDirectory").Return(rootDirectory).Maybe()
	k.On("OsqueryFlags").Return([]string{})
	k.On("OsqueryVerbose").Return(true)
	k.On("LoggingInterval").Return(5 * time.Minute).Maybe()
	k.On("LogMaxBytesPerBatch").Return(0).Maybe()
	k.On("Transport").Return("jsonrpc").Maybe()
	k.On("ReadEnrollSecret").Return("", nil).Maybe()
	setUpMockStores(t, k)
	serviceClient := mockServiceClient()

	runner := New(k, serviceClient)

	// Start the instance
	go runner.Run()
	waitHealthy(t, runner, logBytes)

	// Confirm the default instance was started
	require.Contains(t, runner.instances, types.DefaultRegistrationID)
	require.NotNil(t, runner.instances[types.DefaultRegistrationID].stats)
	require.NotEmpty(t, runner.instances[types.DefaultRegistrationID].stats.StartTime, "start time should be added to default instance stats on start up")
	require.NotEmpty(t, runner.instances[types.DefaultRegistrationID].stats.ConnectTime, "connect time should be added to default instance stats on start up")

	// Confirm the additional instance was started
	require.Contains(t, runner.instances, extraRegistrationId)
	require.NotNil(t, runner.instances[extraRegistrationId].stats)
	require.NotEmpty(t, runner.instances[extraRegistrationId].stats.StartTime, "start time should be added to secondary instance stats on start up")
	require.NotEmpty(t, runner.instances[extraRegistrationId].stats.ConnectTime, "connect time should be added to secondary instance stats on start up")

	// Confirm instance statuses are reported correctly
	instanceStatuses := runner.InstanceStatuses()
	require.Contains(t, instanceStatuses, types.DefaultRegistrationID)
	require.Equal(t, instanceStatuses[types.DefaultRegistrationID], types.InstanceStatusHealthy)
	require.Contains(t, instanceStatuses, extraRegistrationId)
	require.Equal(t, instanceStatuses[extraRegistrationId], types.InstanceStatusHealthy)

	waitShutdown(t, runner, logBytes)

	// Confirm both instances exited
	require.Contains(t, runner.instances, types.DefaultRegistrationID)
	require.NotNil(t, runner.instances[types.DefaultRegistrationID].stats)
	require.NotEmpty(t, runner.instances[types.DefaultRegistrationID].stats.ExitTime, "exit time should be added to default instance stats on shutdown")
	require.Contains(t, runner.instances, extraRegistrationId)
	require.NotNil(t, runner.instances[extraRegistrationId].stats)
	require.NotEmpty(t, runner.instances[extraRegistrationId].stats.ExitTime, "exit time should be added to secondary instance stats on shutdown")
}

func TestRunnerHandlesImmediateShutdownWithMultipleInstances(t *testing.T) {
	t.Parallel()
	rootDirectory := testRootDirectory(t)

	logBytes, slogger := setUpTestSlogger()

	k := typesMocks.NewKnapsack(t)
	k.On("RegistrationIDs").Return([]string{types.DefaultRegistrationID})
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("WatchdogEnabled").Return(false)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("Slogger").Return(slogger)
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinaryDirectory)
	k.On("RootDirectory").Return(rootDirectory).Maybe()
	k.On("OsqueryFlags").Return([]string{})
	k.On("OsqueryVerbose").Return(true)
	k.On("LoggingInterval").Return(5 * time.Minute).Maybe()
	k.On("LogMaxBytesPerBatch").Return(0).Maybe()
	k.On("Transport").Return("jsonrpc").Maybe()
	k.On("ReadEnrollSecret").Return("", nil).Maybe()
	setUpMockStores(t, k)
	serviceClient := mockServiceClient()

	runner := New(k, serviceClient)

	// Add in an extra instance
	extraRegistrationId := ulid.New()
	runner.registrationIds = append(runner.registrationIds, extraRegistrationId)

	// Start the instance
	go runner.Run()

	// Wait very briefly for the launch routines to begin, then shut it down
	time.Sleep(100 * time.Millisecond)
	waitShutdown(t, runner, logBytes)

	// Confirm the default instance was started, and then exited
	require.Contains(t, runner.instances, types.DefaultRegistrationID)
	require.NotNil(t, runner.instances[types.DefaultRegistrationID].stats)
	require.NotEmpty(t, runner.instances[types.DefaultRegistrationID].stats.StartTime, "start time should be added to default instance stats on start up")
	require.NotEmpty(t, runner.instances[types.DefaultRegistrationID].stats.ConnectTime, "connect time should be added to default instance stats on start up")
	require.NotEmpty(t, runner.instances[types.DefaultRegistrationID].stats.ExitTime, "exit time should be added to default instance stats on shutdown")

	// Confirm the additional instance was started, and then exited
	require.Contains(t, runner.instances, extraRegistrationId)
	require.NotNil(t, runner.instances[extraRegistrationId].stats)
	require.NotEmpty(t, runner.instances[extraRegistrationId].stats.StartTime, "start time should be added to secondary instance stats on start up")
	require.NotEmpty(t, runner.instances[extraRegistrationId].stats.ConnectTime, "connect time should be added to secondary instance stats on start up")
	require.NotEmpty(t, runner.instances[extraRegistrationId].stats.ExitTime, "exit time should be added to secondary instance stats on shutdown")
}

func TestMultipleShutdowns(t *testing.T) {
	t.Parallel()
	rootDirectory := testRootDirectory(t)

	logBytes, slogger := setUpTestSlogger()

	k := typesMocks.NewKnapsack(t)
	k.On("RegistrationIDs").Return([]string{types.DefaultRegistrationID})
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("WatchdogEnabled").Return(false)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("Slogger").Return(slogger)
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinaryDirectory)
	k.On("RootDirectory").Return(rootDirectory).Maybe()
	k.On("OsqueryFlags").Return([]string{})
	k.On("OsqueryVerbose").Return(true)
	k.On("LoggingInterval").Return(5 * time.Minute).Maybe()
	k.On("LogMaxBytesPerBatch").Return(0).Maybe()
	k.On("Transport").Return("jsonrpc").Maybe()
	k.On("ReadEnrollSecret").Return("", nil).Maybe()
	setUpMockStores(t, k)

	runner := New(k, mockServiceClient())
	go runner.Run()

	waitHealthy(t, runner, logBytes)

	for i := 0; i < 3; i += 1 {
		waitShutdown(t, runner, logBytes)
	}
}

func TestOsqueryDies(t *testing.T) {
	t.Parallel()
	rootDirectory := testRootDirectory(t)

	logBytes, slogger := setUpTestSlogger()

	k := typesMocks.NewKnapsack(t)
	k.On("RegistrationIDs").Return([]string{types.DefaultRegistrationID})
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("WatchdogEnabled").Return(false)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("Slogger").Return(slogger)
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinaryDirectory)
	k.On("RootDirectory").Return(rootDirectory)
	k.On("OsqueryFlags").Return([]string{})
	k.On("OsqueryVerbose").Return(true)
	k.On("LoggingInterval").Return(5 * time.Minute).Maybe()
	k.On("LogMaxBytesPerBatch").Return(0).Maybe()
	k.On("Transport").Return("jsonrpc").Maybe()
	k.On("ReadEnrollSecret").Return("", nil).Maybe()
	setUpMockStores(t, k)

	runner := New(k, mockServiceClient())
	go runner.Run()

	waitHealthy(t, runner, logBytes)

	require.Contains(t, runner.instances, types.DefaultRegistrationID)
	require.NotNil(t, runner.instances[types.DefaultRegistrationID].stats)
	previousStats := runner.instances[types.DefaultRegistrationID].stats

	// Simulate the osquery process unexpectedly dying
	runner.instanceLock.Lock()
	require.NoError(t, killProcessGroup(runner.instances[types.DefaultRegistrationID].cmd))
	runner.instances[types.DefaultRegistrationID].errgroup.Wait()
	runner.instanceLock.Unlock()

	waitHealthy(t, runner, logBytes)
	require.NotEmpty(t, previousStats.Error, "error should be added to stats when unexpected shutdown")
	require.NotEmpty(t, previousStats.ExitTime, "exit time should be added to instance when unexpected shutdown")

	waitShutdown(t, runner, logBytes)
}

func TestNotStarted(t *testing.T) {
	t.Parallel()
	rootDirectory := t.TempDir()

	k := typesMocks.NewKnapsack(t)
	k.On("RegistrationIDs").Return([]string{types.DefaultRegistrationID})
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("RootDirectory").Return(rootDirectory).Maybe()
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	k.On("Slogger").Return(multislogger.NewNopLogger())
	runner := New(k, mockServiceClient())

	assert.Error(t, runner.Healthy())
	assert.NoError(t, runner.Shutdown())
}

// WithStartFunc defines the function that will be used to exeute the osqueryd
// start command. It is useful during testing to simulate osquery start delays or
// osquery instability.
func WithStartFunc(f func(cmd *exec.Cmd) error) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.startFunc = f
	}
}

// TestExtensionIsCleanedUp tests that the osquery extension cleans
// itself up. Unfortunately, this test has proved very flakey on
// circle-ci, but just fine on laptops.
func TestExtensionIsCleanedUp(t *testing.T) {
	t.Skip("https://github.com/kolide/launcher/issues/478")
	t.Parallel()

	runner, logBytes, teardown := setupOsqueryInstanceForTests(t)
	defer teardown()

	requirePgidMatch(t, runner.instances[types.DefaultRegistrationID].cmd.Process.Pid)

	// kill the current osquery process but not the extension
	require.NoError(t, runner.instances[types.DefaultRegistrationID].cmd.Process.Kill())

	// We need to (a) let the runner restart osquery, and (b) wait for
	// the extension to die. Both of these may take up to 30s. We'll
	// start a clock, wait for the respawn, and after 32s, test that the
	// extension process is no longer running. See
	// https://github.com/kolide/launcher/pull/342 and associated for
	// background.
	timer1 := time.NewTimer(35 * time.Second)

	// Wait for osquery to respawn
	waitHealthy(t, runner, logBytes)

	// Ensure we've waited at least 32s
	<-timer1.C
}

func TestMultipleInstancesWithUpdatedRegistrationIDs(t *testing.T) {
	t.Parallel()
	rootDirectory := testRootDirectory(t)

	logBytes, slogger := setUpTestSlogger()

	k := typesMocks.NewKnapsack(t)
	k.On("RegistrationIDs").Return([]string{types.DefaultRegistrationID})
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("WatchdogEnabled").Return(false)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("Slogger").Return(slogger)
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinaryDirectory)
	k.On("RootDirectory").Return(rootDirectory).Maybe()
	k.On("OsqueryFlags").Return([]string{})
	k.On("OsqueryVerbose").Return(true)
	k.On("LoggingInterval").Return(5 * time.Minute).Maybe()
	k.On("LogMaxBytesPerBatch").Return(0).Maybe()
	k.On("Transport").Return("jsonrpc").Maybe()
	k.On("ReadEnrollSecret").Return("", nil).Maybe()
	setUpMockStores(t, k)
	serviceClient := mockServiceClient()

	runner := New(k, serviceClient)

	// Start the instance
	go runner.Run()
	waitHealthy(t, runner, logBytes)

	// Confirm the default instance was started
	require.Contains(t, runner.instances, types.DefaultRegistrationID)
	require.NotNil(t, runner.instances[types.DefaultRegistrationID].stats)
	require.NotEmpty(t, runner.instances[types.DefaultRegistrationID].stats.StartTime, "start time should be added to default instance stats on start up")
	require.NotEmpty(t, runner.instances[types.DefaultRegistrationID].stats.ConnectTime, "connect time should be added to default instance stats on start up")

	// confirm only the default instance has started
	require.Equal(t, 1, len(runner.instances))

	// Confirm instance statuses are reported correctly
	instanceStatuses := runner.InstanceStatuses()
	require.Contains(t, instanceStatuses, types.DefaultRegistrationID)
	require.Equal(t, instanceStatuses[types.DefaultRegistrationID], types.InstanceStatusHealthy)

	// Add in an extra instance
	extraRegistrationId := ulid.New()
	updateErr := runner.UpdateRegistrationIDs([]string{types.DefaultRegistrationID, extraRegistrationId})
	require.NoError(t, updateErr)
	waitHealthy(t, runner, logBytes)
	updatedInstanceStatuses := runner.InstanceStatuses()
	// verify that rerunRequired has been reset for any future changes
	require.False(t, runner.rerunRequired.Load())
	// now verify both instances are reported
	require.Equal(t, 2, len(runner.instances))
	require.Contains(t, updatedInstanceStatuses, types.DefaultRegistrationID)
	require.Contains(t, updatedInstanceStatuses, extraRegistrationId)
	// Confirm the additional instance was started and is healthy
	require.NotNil(t, runner.instances[extraRegistrationId].stats)
	require.NotEmpty(t, runner.instances[extraRegistrationId].stats.StartTime, "start time should be added to secondary instance stats on start up")
	require.NotEmpty(t, runner.instances[extraRegistrationId].stats.ConnectTime, "connect time should be added to secondary instance stats on start up")
	require.Equal(t, updatedInstanceStatuses[extraRegistrationId], types.InstanceStatusHealthy)

	// update registration IDs one more time, this time removing the additional registration
	originalDefaultInstanceStartTime := runner.instances[extraRegistrationId].stats.StartTime
	updateErr = runner.UpdateRegistrationIDs([]string{types.DefaultRegistrationID})
	require.NoError(t, updateErr)
	waitHealthy(t, runner, logBytes)

	// now verify only the default instance remains
	require.Equal(t, 1, len(runner.instances))
	// Confirm the default instance was started and is healthy
	require.Contains(t, runner.instances, types.DefaultRegistrationID)
	require.NotNil(t, runner.instances[types.DefaultRegistrationID].stats)
	require.NotEmpty(t, runner.instances[types.DefaultRegistrationID].stats.StartTime, "start time should be added to default instance stats on start up")
	require.NotEmpty(t, runner.instances[types.DefaultRegistrationID].stats.ConnectTime, "connect time should be added to default instance stats on start up")
	// verify that rerunRequired has been reset for any future changes
	require.False(t, runner.rerunRequired.Load())
	// verify the default instance was restarted
	require.NotEqual(t, originalDefaultInstanceStartTime, runner.instances[types.DefaultRegistrationID].stats.StartTime)

	waitShutdown(t, runner, logBytes)

	// Confirm both instances exited
	require.Contains(t, runner.instances, types.DefaultRegistrationID)
	require.NotNil(t, runner.instances[types.DefaultRegistrationID].stats)
	require.NotEmpty(t, runner.instances[types.DefaultRegistrationID].stats.ExitTime, "exit time should be added to default instance stats on shutdown")
}

func TestUpdatingRegistrationIDsOnlyRestartsForChanges(t *testing.T) {
	t.Parallel()
	rootDirectory := testRootDirectory(t)

	logBytes, slogger := setUpTestSlogger()
	extraRegistrationId := ulid.New()

	k := typesMocks.NewKnapsack(t)
	k.On("RegistrationIDs").Return([]string{types.DefaultRegistrationID, extraRegistrationId})
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("WatchdogEnabled").Return(false)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("Slogger").Return(slogger)
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinaryDirectory)
	k.On("RootDirectory").Return(rootDirectory).Maybe()
	k.On("OsqueryFlags").Return([]string{})
	k.On("OsqueryVerbose").Return(true)
	k.On("LoggingInterval").Return(5 * time.Minute).Maybe()
	k.On("LogMaxBytesPerBatch").Return(0).Maybe()
	k.On("Transport").Return("jsonrpc").Maybe()
	k.On("ReadEnrollSecret").Return("", nil).Maybe()
	setUpMockStores(t, k)
	serviceClient := mockServiceClient()

	runner := New(k, serviceClient)

	// Start the instance
	go runner.Run()
	waitHealthy(t, runner, logBytes)

	require.Equal(t, 2, len(runner.instances))
	// Confirm the default instance was started
	require.Contains(t, runner.instances, types.DefaultRegistrationID)
	require.NotNil(t, runner.instances[types.DefaultRegistrationID].stats)
	require.NotEmpty(t, runner.instances[types.DefaultRegistrationID].stats.StartTime, "start time should be added to default instance stats on start up")
	require.NotEmpty(t, runner.instances[types.DefaultRegistrationID].stats.ConnectTime, "connect time should be added to default instance stats on start up")
	// note the original start time
	defaultInstanceStartTime := runner.instances[types.DefaultRegistrationID].stats.StartTime

	// Confirm the extra instance was started
	require.Contains(t, runner.instances, extraRegistrationId)
	require.NotNil(t, runner.instances[extraRegistrationId].stats)
	require.NotEmpty(t, runner.instances[extraRegistrationId].stats.StartTime, "start time should be added to extra instance stats on start up")
	require.NotEmpty(t, runner.instances[extraRegistrationId].stats.ConnectTime, "connect time should be added to extra instance stats on start up")
	// note the original start time
	extraInstanceStartTime := runner.instances[extraRegistrationId].stats.StartTime

	// rerun with identical registrationIDs in swapped order and verify that the instances are not restarted
	updateErr := runner.UpdateRegistrationIDs([]string{extraRegistrationId, types.DefaultRegistrationID})
	require.NoError(t, updateErr)
	waitHealthy(t, runner, logBytes)

	require.Equal(t, 2, len(runner.instances))
	require.Equal(t, extraInstanceStartTime, runner.instances[extraRegistrationId].stats.StartTime)
	require.Equal(t, defaultInstanceStartTime, runner.instances[types.DefaultRegistrationID].stats.StartTime)

	waitShutdown(t, runner, logBytes)

	// Confirm both instances exited
	require.Contains(t, runner.instances, types.DefaultRegistrationID)
	require.NotNil(t, runner.instances[types.DefaultRegistrationID].stats)
	require.NotEmpty(t, runner.instances[types.DefaultRegistrationID].stats.ExitTime, "exit time should be added to default instance stats on shutdown")
	require.Contains(t, runner.instances, extraRegistrationId)
	require.NotNil(t, runner.instances[extraRegistrationId].stats)
	require.NotEmpty(t, runner.instances[extraRegistrationId].stats.ExitTime, "exit time should be added to secondary instance stats on shutdown")
}

// sets up an osquery instance with a running extension to be used in tests.
func setupOsqueryInstanceForTests(t *testing.T) (runner *Runner, logBytes *threadsafebuffer.ThreadSafeBuffer, teardown func()) {
	rootDirectory := testRootDirectory(t)

	logBytes, slogger := setUpTestSlogger()

	k := typesMocks.NewKnapsack(t)
	k.On("RegistrationIDs").Return([]string{types.DefaultRegistrationID})
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("WatchdogEnabled").Return(true)
	k.On("WatchdogMemoryLimitMB").Return(150)
	k.On("WatchdogUtilizationLimitPercent").Return(20)
	k.On("WatchdogDelaySec").Return(120)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
	k.On("Slogger").Return(slogger)
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinaryDirectory)
	k.On("RootDirectory").Return(rootDirectory).Maybe()
	k.On("OsqueryFlags").Return([]string{}).Maybe()
	k.On("OsqueryVerbose").Return(true).Maybe()
	k.On("LoggingInterval").Return(5 * time.Minute).Maybe()
	k.On("LogMaxBytesPerBatch").Return(0).Maybe()
	k.On("Transport").Return("jsonrpc").Maybe()
	k.On("ReadEnrollSecret").Return("", nil).Maybe()
	setUpMockStores(t, k)

	runner = New(k, mockServiceClient())
	go runner.Run()
	waitHealthy(t, runner, logBytes)

	requirePgidMatch(t, runner.instances[types.DefaultRegistrationID].cmd.Process.Pid)

	teardown = func() {
		waitShutdown(t, runner, logBytes)
	}
	return runner, logBytes, teardown
}

// setUpMockStores creates test stores in the test knapsack
func setUpMockStores(t *testing.T, k *typesMocks.Knapsack) {
	store, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.KatcConfigStore.String())
	require.NoError(t, err)
	k.On("KatcConfigStore").Return(store).Maybe()
	k.On("ConfigStore").Return(inmemory.NewStore()).Maybe()
	k.On("LauncherHistoryStore").Return(inmemory.NewStore()).Maybe()
	k.On("ServerProvidedDataStore").Return(inmemory.NewStore()).Maybe()
	k.On("AgentFlagsStore").Return(inmemory.NewStore()).Maybe()
	k.On("AutoupdateErrorsStore").Return(inmemory.NewStore()).Maybe()
	k.On("StatusLogsStore").Return(inmemory.NewStore()).Maybe()
	k.On("ResultLogsStore").Return(inmemory.NewStore()).Maybe()
	k.On("BboltDB").Return(storageci.SetupDB(t)).Maybe()
}

// mockServiceClient returns a mock KolideService that returns the minimum possible response
// for all methods.
func mockServiceClient() *servicemock.KolideService {
	return &servicemock.KolideService{
		RequestEnrollmentFunc: func(ctx context.Context, enrollSecret, hostIdentifier string, details service.EnrollmentDetails) (string, bool, error) {
			return "testnodekey", false, nil
		},
		RequestConfigFunc: func(ctx context.Context, nodeKey string) (string, bool, error) {
			return "", false, errors.New("transport")
		},
		PublishLogsFunc: func(ctx context.Context, nodeKey string, logType logger.LogType, logs []string) (string, string, bool, error) {
			return "", "", false, nil
		},
		RequestQueriesFunc: func(ctx context.Context, nodeKey string) (*distributed.GetQueriesResult, bool, error) {
			return nil, false, errors.New("transport")
		},
		PublishResultsFunc: func(ctx context.Context, nodeKey string, results []distributed.Result) (string, string, bool, error) {
			return "", "", false, nil
		},
		CheckHealthFunc: func(ctx context.Context) (int32, error) {
			return 0, nil
		},
	}
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
	if runtime.GOOS == "windows" {
		return t.TempDir()
	}

	ulid := ulid.New()
	rootDir := filepath.Join(os.TempDir(), ulid[len(ulid)-4:])
	require.NoError(t, os.Mkdir(rootDir, 0700))

	t.Cleanup(func() {
		if err := os.RemoveAll(rootDir); err != nil {
			t.Errorf("testRootDirectory RemoveAll cleanup: %v", err)
		}
	})

	return rootDir
}
