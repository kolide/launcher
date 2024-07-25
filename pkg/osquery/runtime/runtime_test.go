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
	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	typesMocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/backoff"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/osquery/runtime/history"
	"github.com/kolide/launcher/pkg/packaging"
	"github.com/kolide/launcher/pkg/threadsafebuffer"

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

	binDirectory, rmBinDirectory, err := osqueryTempDir()
	if err != nil {
		fmt.Println("Failed to make temp dir for test binaries")
		os.Exit(1) //nolint:forbidigo // Fine to use os.Exit in tests
	}
	defer rmBinDirectory()

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

// getBinDir finds the directory of the currently running binary (where we will
// look for the osquery extension)
func getBinDir() (string, error) {
	binPath, err := os.Executable()
	if err != nil {
		return "", err
	}
	binDir := filepath.Dir(binPath)
	return binDir, nil
}

func TestCalculateOsqueryPaths(t *testing.T) {
	t.Parallel()
	binDir, err := getBinDir()
	require.NoError(t, err)

	paths, err := calculateOsqueryPaths(osqueryOptions{
		rootDirectory: binDir,
	})

	require.NoError(t, err)

	// ensure that all of our resulting artifact files are in the rootDir that we
	// dictated
	require.Equal(t, binDir, filepath.Dir(paths.pidfilePath))
	require.Equal(t, binDir, filepath.Dir(paths.databasePath))

	// socket path on windows includes semi-random ulid
	if runtime.GOOS != "windows" {
		require.Equal(t, binDir, filepath.Dir(paths.extensionSocketPath))
	}

	require.Equal(t, binDir, filepath.Dir(paths.extensionAutoloadPath))
}

func TestCreateOsqueryCommand(t *testing.T) {
	t.Parallel()
	paths := &osqueryFilePaths{
		pidfilePath:           "/foo/bar/osquery-abcd.pid",
		databasePath:          "/foo/bar/osquery.db",
		extensionSocketPath:   "/foo/bar/osquery.sock",
		extensionAutoloadPath: "/foo/bar/osquery.autoload",
	}

	osquerydPath := testOsqueryBinaryDirectory

	osqOpts := &osqueryOptions{
		configPluginFlag:      "config_plugin",
		loggerPluginFlag:      "logger_plugin",
		distributedPluginFlag: "distributed_plugin",
		stdout:                os.Stdout,
		stderr:                os.Stderr,
	}
	k := typesMocks.NewKnapsack(t)
	k.On("WatchdogEnabled").Return(true)
	k.On("WatchdogMemoryLimitMB").Return(150)
	k.On("WatchdogUtilizationLimitPercent").Return(20)
	k.On("WatchdogDelaySec").Return(120)

	i := newInstance()
	i.opts = *osqOpts
	i.knapsack = k

	cmd, err := i.createOsquerydCommand(osquerydPath, paths)
	require.NoError(t, err)
	require.Equal(t, os.Stderr, cmd.Stderr)
	require.Equal(t, os.Stdout, cmd.Stdout)

	k.AssertExpectations(t)
}

func TestCreateOsqueryCommandWithFlags(t *testing.T) {
	t.Parallel()
	osqOpts := &osqueryOptions{
		osqueryFlags: []string{"verbose=false", "windows_event_channels=foo,bar"},
	}
	k := typesMocks.NewKnapsack(t)
	k.On("WatchdogEnabled").Return(true)
	k.On("WatchdogMemoryLimitMB").Return(150)
	k.On("WatchdogUtilizationLimitPercent").Return(20)
	k.On("WatchdogDelaySec").Return(120)

	i := newInstance()
	i.opts = *osqOpts
	i.knapsack = k

	cmd, err := i.createOsquerydCommand(
		testOsqueryBinaryDirectory,
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

	osqOpts := &osqueryOptions{}
	k := typesMocks.NewKnapsack(t)
	k.On("WatchdogEnabled").Return(true)
	k.On("WatchdogMemoryLimitMB").Return(150)
	k.On("WatchdogUtilizationLimitPercent").Return(20)
	k.On("WatchdogDelaySec").Return(120)

	i := newInstance()
	i.opts = *osqOpts
	i.knapsack = k

	cmd, err := i.createOsquerydCommand(
		testOsqueryBinaryDirectory,
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

	osqOpts := &osqueryOptions{}
	k := typesMocks.NewKnapsack(t)
	k.On("WatchdogEnabled").Return(false)

	i := newInstance()
	i.opts = *osqOpts
	i.knapsack = k

	cmd, err := i.createOsquerydCommand(
		testOsqueryBinaryDirectory,
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

// downloadOsqueryInBinDir downloads osqueryd. This allows the test
// suite to run on hosts lacking osqueryd. We could consider moving this into a deps step.
func downloadOsqueryInBinDir(binDirectory string) error {
	target := packaging.Target{}
	if err := target.PlatformFromString(runtime.GOOS); err != nil {
		return fmt.Errorf("Error parsing platform: %s: %w", runtime.GOOS, err)
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
	rootDirectory, rmRootDirectory, err := osqueryTempDir()
	require.NoError(t, err)
	defer rmRootDirectory()

	k := typesMocks.NewKnapsack(t)
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("WatchdogEnabled").Return(false)
	k.On("Slogger").Return(multislogger.NewNopLogger())
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("LatestOsquerydPath", mock.Anything).Return("")

	runner := New(
		k,
		WithKnapsack(k),
		WithRootDirectory(rootDirectory),
	)
	assert.Error(t, runner.Run())

	k.AssertExpectations(t)
}

func TestWithOsqueryFlags(t *testing.T) {
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
	k.On("KatcConfigStore").Return(store).Maybe() // attempt to make this test less flaky

	runner := New(
		k,
		WithKnapsack(k),
		WithRootDirectory(rootDirectory),
		WithOsqueryFlags([]string{"verbose=false"}),
	)
	go runner.Run()
	waitHealthy(t, runner, &logBytes)
	assert.Equal(t, []string{"verbose=false"}, runner.instance.opts.osqueryFlags)

	waitShutdown(t, runner, &logBytes)
}

func TestFlagsChanged(t *testing.T) {
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
	// First, it should return false, then on the next call, it should return true
	k.On("WatchdogEnabled").Return(false).Once()
	k.On("WatchdogEnabled").Return(true).Once()
	k.On("WatchdogMemoryLimitMB").Return(150)
	k.On("WatchdogUtilizationLimitPercent").Return(20)
	k.On("WatchdogDelaySec").Return(120)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("Slogger").Return(slogger)
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinaryDirectory)
	store, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.KatcConfigStore.String())
	require.NoError(t, err)
	k.On("KatcConfigStore").Return(store).Maybe() // attempt to make this test less flaky

	// Start the runner
	runner := New(
		k,
		WithKnapsack(k),
		WithRootDirectory(rootDirectory),
		WithOsqueryFlags([]string{"verbose=false"}),
	)
	go runner.Run()

	// Wait for the instance to start
	time.Sleep(2 * time.Second)
	waitHealthy(t, runner, &logBytes)

	// Confirm watchdog is disabled
	watchdogDisabled := false
	for _, a := range runner.instance.cmd.Args {
		if strings.Contains(a, "disable_watchdog") {
			watchdogDisabled = true
			break
		}
	}
	require.True(t, watchdogDisabled, "instance not set up with watchdog disabled")

	startingInstance := runner.instance

	runner.FlagsChanged(keys.WatchdogEnabled)

	// Wait for the instance to restart
	time.Sleep(2 * time.Second)
	waitHealthy(t, runner, &logBytes)

	// Now confirm that the instance is new
	require.NotEqual(t, startingInstance, runner.instance, "instance not replaced")

	// Confirm osquery watchdog is now enabled
	watchdogMemoryLimitMBFound := false
	watchdogUtilizationLimitPercentFound := false
	watchdogDelaySecFound := false
	for _, a := range runner.instance.cmd.Args {
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

	waitShutdown(t, runner, &logBytes)
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
		require.NoError(t, err, fmt.Sprintf("runner logs: %s", logBytes.String()))
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

		// Confirms osquery instance setup is complete
		if runner.instance != nil && runner.instance.stats.ConnectTime == "" {
			return errors.New("no connect time set yet")
		}

		// Good to go
		return nil
	}, 30*time.Second, 1*time.Second), fmt.Sprintf("runner logs: %s", logBytes.String()))

	// Give the instance just a little bit of buffer before we proceed
	time.Sleep(2 * time.Second)
}

func TestSimplePath(t *testing.T) {
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
	k.On("KatcConfigStore").Return(store).Maybe() // attempt to make this test less flaky

	runner := New(
		k,
		WithKnapsack(k),
		WithRootDirectory(rootDirectory),
	)
	go runner.Run()

	waitHealthy(t, runner, &logBytes)

	require.NotEmpty(t, runner.instance.stats.StartTime, "start time should be added to instance stats on start up")
	require.NotEmpty(t, runner.instance.stats.ConnectTime, "connect time should be added to instance stats on start up")

	waitShutdown(t, runner, &logBytes)
}

func TestMultipleShutdowns(t *testing.T) {
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
	k.On("KatcConfigStore").Return(store).Maybe() // attempt to make this test less flaky

	runner := New(
		k,
		WithKnapsack(k),
		WithRootDirectory(rootDirectory),
	)
	go runner.Run()

	waitHealthy(t, runner, &logBytes)

	for i := 0; i < 3; i += 1 {
		waitShutdown(t, runner, &logBytes)
	}
}

func TestOsqueryDies(t *testing.T) {
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
	k.On("KatcConfigStore").Return(store).Maybe() // attempt to make this test less flaky

	runner := New(
		k,
		WithKnapsack(k),
		WithRootDirectory(rootDirectory),
	)
	go runner.Run()
	require.NoError(t, err)

	waitHealthy(t, runner, &logBytes)

	previousStats := runner.instance.stats

	// Simulate the osquery process unexpectedly dying
	runner.instanceLock.Lock()
	require.NoError(t, killProcessGroup(runner.instance.cmd))
	runner.instance.errgroup.Wait()
	runner.instanceLock.Unlock()

	waitHealthy(t, runner, &logBytes)
	require.NotEmpty(t, previousStats.Error, "error should be added to stats when unexpected shutdown")
	require.NotEmpty(t, previousStats.ExitTime, "exit time should be added to instance when unexpected shutdown")

	waitShutdown(t, runner, &logBytes)
}

func TestNotStarted(t *testing.T) {
	t.Parallel()
	rootDirectory, rmRootDirectory, err := osqueryTempDir()
	require.NoError(t, err)
	defer rmRootDirectory()

	k := typesMocks.NewKnapsack(t)
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	runner := newRunner(WithKnapsack(k), WithRootDirectory(rootDirectory))
	require.NoError(t, err)

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

	requirePgidMatch(t, runner.instance.cmd.Process.Pid)

	// kill the current osquery process but not the extension
	require.NoError(t, runner.instance.cmd.Process.Kill())

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

// sets up an osquery instance with a running extension to be used in tests.
func setupOsqueryInstanceForTests(t *testing.T) (runner *Runner, logBytes *threadsafebuffer.ThreadSafeBuffer, teardown func()) {
	rootDirectory, rmRootDirectory, err := osqueryTempDir()
	require.NoError(t, err)

	logBytes = &threadsafebuffer.ThreadSafeBuffer{}
	slogger := slog.New(slog.NewTextHandler(logBytes, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}))

	k := typesMocks.NewKnapsack(t)
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("WatchdogEnabled").Return(true)
	k.On("WatchdogMemoryLimitMB").Return(150)
	k.On("WatchdogUtilizationLimitPercent").Return(20)
	k.On("WatchdogDelaySec").Return(120)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
	k.On("Slogger").Return(slogger)
	k.On("LatestOsquerydPath", mock.Anything).Return(testOsqueryBinaryDirectory)
	store, err := storageci.NewStore(t, multislogger.NewNopLogger(), storage.KatcConfigStore.String())
	require.NoError(t, err)
	k.On("KatcConfigStore").Return(store)

	runner = New(
		k,
		WithKnapsack(k),
		WithRootDirectory(rootDirectory),
	)
	go runner.Run()
	waitHealthy(t, runner, logBytes)

	requirePgidMatch(t, runner.instance.cmd.Process.Pid)

	teardown = func() {
		defer rmRootDirectory()
		waitShutdown(t, runner, logBytes)
	}
	return runner, logBytes, teardown
}
