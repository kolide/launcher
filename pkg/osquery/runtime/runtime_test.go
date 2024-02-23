//go:build !windows
// +build !windows

package runtime

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
	"syscall"
	"testing"
	"time"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/kolide/kit/fsutil"
	"github.com/kolide/kit/testutil"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	typesMocks "github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/osquery/runtime/history"
	"github.com/kolide/launcher/pkg/packaging"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	osquery "github.com/osquery/osquery-go"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var testOsqueryBinaryDirectory string

// TestMain overrides the default test main function. This allows us to share setup/teardown.
func TestMain(m *testing.M) {
	binDirectory, rmBinDirectory, err := osqueryTempDir()
	if err != nil {
		fmt.Println("Failed to make temp dir for test binaries")
		os.Exit(1)
	}
	defer rmBinDirectory()

	s, err := storageci.NewStore(nil, multislogger.NewNopLogger(), storage.OsqueryHistoryInstanceStore.String())
	if err != nil {
		fmt.Println("Failed to make new store")
		os.Exit(1)
	}
	if err := history.InitHistory(s); err != nil {
		fmt.Println("Failed to init history")
		os.Exit(1)
	}

	testOsqueryBinaryDirectory = filepath.Join(binDirectory, "osqueryd")

	thrift.ServerConnectivityCheckInterval = 100 * time.Millisecond

	if err := downloadOsqueryInBinDir(binDirectory); err != nil {
		fmt.Printf("Failed to download osquery: %v\n", err)
		os.Exit(1)
	}

	// Run the tests!
	retCode := m.Run()
	os.Exit(retCode)
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
	require.Equal(t, binDir, filepath.Dir(paths.extensionSocketPath))
	require.Equal(t, binDir, filepath.Dir(paths.extensionAutoloadPath))
}

func TestCreateOsqueryCommand(t *testing.T) {
	t.Parallel()
	paths := &osqueryFilePaths{
		pidfilePath:           "/foo/bar/osquery.pid",
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
	cacheDir := "/tmp"

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

	runner := New(
		k,
		WithKnapsack(k),
		WithRootDirectory(rootDirectory),
		WithOsquerydBinary("/foobar"),
	)
	assert.Error(t, runner.Run())

	k.AssertExpectations(t)
}

func TestWithOsqueryFlags(t *testing.T) {
	t.Parallel()
	rootDirectory, rmRootDirectory, err := osqueryTempDir()
	require.NoError(t, err)
	defer rmRootDirectory()

	k := typesMocks.NewKnapsack(t)
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("WatchdogEnabled").Return(false)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("Slogger").Return(multislogger.NewNopLogger())

	runner := New(
		k,
		WithKnapsack(k),
		WithRootDirectory(rootDirectory),
		WithOsquerydBinary(testOsqueryBinaryDirectory),
		WithOsqueryFlags([]string{"verbose=false"}),
	)
	go runner.Run()
	waitHealthy(t, runner)
	assert.Equal(t, []string{"verbose=false"}, runner.instance.opts.osqueryFlags)

	runner.Interrupt(errors.New("test error"))
}

func TestFlagsChanged(t *testing.T) {
	t.Parallel()

	rootDirectory, rmRootDirectory, err := osqueryTempDir()
	require.NoError(t, err)
	defer rmRootDirectory()

	k := typesMocks.NewKnapsack(t)
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	// First, it should return false, then on the next call, it should return true
	k.On("WatchdogEnabled").Return(false).Once()
	k.On("WatchdogEnabled").Return(true).Once()
	k.On("WatchdogMemoryLimitMB").Return(150)
	k.On("WatchdogUtilizationLimitPercent").Return(20)
	k.On("WatchdogDelaySec").Return(120)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("Slogger").Return(multislogger.NewNopLogger())

	// Start the runner
	runner := New(
		k,
		WithKnapsack(k),
		WithRootDirectory(rootDirectory),
		WithOsquerydBinary(testOsqueryBinaryDirectory),
		WithOsqueryFlags([]string{"verbose=false"}),
	)
	go runner.Run()

	// Wait for the instance to start
	time.Sleep(2 * time.Second)
	waitHealthy(t, runner)

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
	waitHealthy(t, runner)

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

	runner.Interrupt(errors.New("test error"))
}

// waitHealthy expects the instance to be healthy within 30 seconds, or else
// fatals the test
func waitHealthy(t *testing.T, runner *Runner) {
	testutil.FatalAfterFunc(t, 30*time.Second, func() {
		for runner.Healthy() != nil {
			time.Sleep(500 * time.Millisecond)
		}
	})
}

func TestSimplePath(t *testing.T) {
	t.Parallel()
	rootDirectory, rmRootDirectory, err := osqueryTempDir()
	require.NoError(t, err)
	defer rmRootDirectory()

	k := typesMocks.NewKnapsack(t)
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("WatchdogEnabled").Return(false)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("Slogger").Return(multislogger.NewNopLogger())

	runner := New(
		k,
		WithKnapsack(k),
		WithRootDirectory(rootDirectory),
		WithOsquerydBinary(testOsqueryBinaryDirectory),
	)
	go runner.Run()

	waitHealthy(t, runner)

	require.NotEmpty(t, runner.instance.stats.StartTime, "start time should be added to instance stats on start up")
	require.NotEmpty(t, runner.instance.stats.ConnectTime, "connect time should be added to instance stats on start up")

	require.NoError(t, runner.Shutdown())
}

func TestMultipleShutdowns(t *testing.T) {
	t.Parallel()
	rootDirectory, rmRootDirectory, err := osqueryTempDir()
	require.NoError(t, err)
	defer rmRootDirectory()

	k := typesMocks.NewKnapsack(t)
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("WatchdogEnabled").Return(false)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("Slogger").Return(multislogger.NewNopLogger())

	runner := New(
		k,
		WithKnapsack(k),
		WithRootDirectory(rootDirectory),
		WithOsquerydBinary(testOsqueryBinaryDirectory),
	)
	go runner.Run()

	waitHealthy(t, runner)

	for i := 0; i < 3; i += 1 {
		require.NoError(t, runner.Shutdown(), "expected no error on calling shutdown but received error on attempt: ", i)
	}
}

func TestRestart(t *testing.T) {
	t.Parallel()
	runner, teardown := setupOsqueryInstanceForTests(t)
	defer teardown()

	previousStats := runner.instance.stats

	require.NoError(t, runner.Restart())
	waitHealthy(t, runner)

	require.NotEmpty(t, runner.instance.stats.StartTime, "start time should be set on latest instance stats after restart")
	require.NotEmpty(t, runner.instance.stats.ConnectTime, "connect time should be set on latest instance stats after restart")

	require.NotEmpty(t, previousStats.ExitTime, "exit time should be set on last instance stats when restarted")
	require.NotEmpty(t, previousStats.Error, "stats instance should have an error on restart")

	previousStats = runner.instance.stats

	require.NoError(t, runner.Restart())
	waitHealthy(t, runner)

	require.NotEmpty(t, runner.instance.stats.StartTime, "start time should be added to latest instance stats after restart")
	require.NotEmpty(t, runner.instance.stats.ConnectTime, "connect time should be added to latest instance stats after restart")

	require.NotEmpty(t, previousStats.ExitTime, "exit time should be set on instance stats when restarted")
	require.NotEmpty(t, previousStats.Error, "stats instance should have an error on restart")
}

func TestOsqueryDies(t *testing.T) {
	t.Parallel()
	rootDirectory, rmRootDirectory, err := osqueryTempDir()
	require.NoError(t, err)
	defer rmRootDirectory()

	k := typesMocks.NewKnapsack(t)
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("WatchdogEnabled").Return(false)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	k.On("Slogger").Return(multislogger.NewNopLogger())

	runner := New(
		k,
		WithKnapsack(k),
		WithRootDirectory(rootDirectory),
		WithOsquerydBinary(testOsqueryBinaryDirectory),
	)
	go runner.Run()
	require.NoError(t, err)

	waitHealthy(t, runner)

	previousStats := runner.instance.stats

	// Simulate the osquery process unexpectedly dying
	runner.instanceLock.Lock()
	require.NoError(t, killProcessGroup(runner.instance.cmd))
	runner.instance.errgroup.Wait()
	runner.instanceLock.Unlock()

	waitHealthy(t, runner)
	require.NotEmpty(t, previousStats.Error, "error should be added to stats when unexpected shutdown")
	require.NotEmpty(t, previousStats.ExitTime, "exit time should be added to instance when unexpected shutdown")

	require.NoError(t, runner.Shutdown())
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

// TestExtensionIsCleanedUp tests that the osquery extension cleans
// itself up. Unfortunately, this test has proved very flakey on
// circle-ci, but just fine on laptops.
func TestExtensionIsCleanedUp(t *testing.T) {
	t.Skip("https://github.com/kolide/launcher/issues/478")
	t.Parallel()

	runner, teardown := setupOsqueryInstanceForTests(t)
	defer teardown()

	osqueryPID := runner.instance.cmd.Process.Pid

	pgid, err := syscall.Getpgid(osqueryPID)
	require.NoError(t, err)
	require.Equal(t, pgid, osqueryPID, "pgid must be set")

	require.NoError(t, err)

	// kill the current osquery process but not the extension
	err = syscall.Kill(osqueryPID, syscall.SIGKILL)
	require.NoError(t, err)

	// We need to (a) let the runner restart osquery, and (b) wait for
	// the extension to die. Both of these may take up to 30s. We'll
	// start a clock, wait for the respawn, and after 32s, test that the
	// extension process is no longer running. See
	// https://github.com/kolide/launcher/pull/342 and associated for
	// background.
	timer1 := time.NewTimer(35 * time.Second)

	// Wait for osquery to respawn
	waitHealthy(t, runner)

	// Ensure we've waited at least 32s
	<-timer1.C
}

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

// WithStartFunc defines the function that will be used to exeute the osqueryd
// start command. It is useful during testing to simulate osquery start delays or
// osquery instability.
func WithStartFunc(f func(cmd *exec.Cmd) error) OsqueryInstanceOption {
	return func(i *OsqueryInstance) {
		i.startFunc = f
	}
}

// sets up an osquery instance with a running extension to be used in tests.
func setupOsqueryInstanceForTests(t *testing.T) (runner *Runner, teardown func()) {
	rootDirectory, rmRootDirectory, err := osqueryTempDir()
	require.NoError(t, err)

	k := typesMocks.NewKnapsack(t)
	k.On("OsqueryHealthcheckStartupDelay").Return(0 * time.Second).Maybe()
	k.On("WatchdogEnabled").Return(true)
	k.On("WatchdogMemoryLimitMB").Return(150)
	k.On("WatchdogUtilizationLimitPercent").Return(20)
	k.On("WatchdogDelaySec").Return(120)
	k.On("RegisterChangeObserver", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
	k.On("Slogger").Return(multislogger.NewNopLogger())

	runner = New(
		k,
		WithKnapsack(k),
		WithRootDirectory(rootDirectory),
		WithOsquerydBinary(testOsqueryBinaryDirectory),
	)
	go runner.Run()
	waitHealthy(t, runner)

	osqueryPID := runner.instance.cmd.Process.Pid

	pgid, err := syscall.Getpgid(osqueryPID)
	require.NoError(t, err)
	require.Equal(t, pgid, osqueryPID)

	teardown = func() {
		defer rmRootDirectory()
		require.NoError(t, runner.Shutdown())
	}
	return runner, teardown
}
