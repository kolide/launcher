//go:build !windows
// +build !windows

package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kolide/kit/fsutil"
	"github.com/kolide/kit/testutil"
	"github.com/kolide/launcher/pkg/osquery/runtime/history"
	"github.com/kolide/launcher/pkg/packaging"
	osquery "github.com/osquery/osquery-go"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
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

	db, err := bbolt.Open(filepath.Join(binDirectory, "osquery_instance_history_test.db"), 0600, &bbolt.Options{
		Timeout: 1 * time.Second,
	})
	if err != nil {
		fmt.Println("Falied to create bolt db")
		os.Exit(1)
	}
	if err := history.InitHistory(db); err != nil {
		fmt.Println("Failed to init history")
		os.Exit(1)
	}

	testOsqueryBinaryDirectory = filepath.Join(binDirectory, "osqueryd")

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

func TestCalculateOsqueryPathsWithAutoloadedExtensions(t *testing.T) {
	t.Parallel()
	binDir, err := getBinDir()
	require.NoError(t, err)

	extensionPaths := make([]string, 0)

	for _, extension := range []string{"extensionInExecDir1", "extensionInExecDir2"} {
		// create file at each extension path
		extensionPath := filepath.Join(binDir, extension)
		require.NoError(t, os.WriteFile(extensionPath, []byte("{}"), 0644))
		extensionPaths = append(extensionPaths, extensionPath)
	}

	nonExecDir := t.TempDir()
	for _, extension := range []string{"extensionNotInExecDir1", "extensionNotInExecDir2"} {
		// create file at each extension path
		extensionPath := filepath.Join(nonExecDir, extension)
		require.NoError(t, os.WriteFile(extensionPath, []byte("{}"), 0644))
		extensionPaths = append(extensionPaths, extensionPath)
	}

	paths, err := calculateOsqueryPaths(osqueryOptions{
		rootDirectory:        binDir,
		autoloadedExtensions: []string{"extensionInExecDir1", "extensionInExecDir2", filepath.Join(nonExecDir, "extensionNotInExecDir1"), filepath.Join(nonExecDir, "extensionNotInExecDir2")},
	})

	require.NoError(t, err)

	// ensure that all of our resulting artifact files are in the rootDir that we
	// dictated
	require.Equal(t, binDir, filepath.Dir(paths.pidfilePath))
	require.Equal(t, binDir, filepath.Dir(paths.databasePath))
	require.Equal(t, binDir, filepath.Dir(paths.extensionSocketPath))
	require.Equal(t, binDir, filepath.Dir(paths.extensionAutoloadPath))

	osqueryAutoloadFilePath := filepath.Join(binDir, "osquery.autoload")
	// read each line of the autoload file into a string array
	bytes, err := os.ReadFile(osqueryAutoloadFilePath)
	require.NoError(t, err)
	autoloadFileLines := strings.Split(string(bytes), "\n")

	// add empty string to extensions path array so it matches the last line of autoloaded file
	assert.ElementsMatch(t, append(extensionPaths, ""), autoloadFileLines)
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
	cmd, err := osqOpts.createOsquerydCommand(osquerydPath, paths)
	require.NoError(t, err)
	require.Equal(t, os.Stderr, cmd.Stderr)
	require.Equal(t, os.Stdout, cmd.Stdout)
}

func TestCreateOsqueryCommandWithFlags(t *testing.T) {
	t.Parallel()
	osqOpts := &osqueryOptions{
		osqueryFlags: []string{"verbose=false", "windows_event_channels=foo,bar"},
	}
	cmd, err := osqOpts.createOsquerydCommand(
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
}

// downloadOsqueryInBinDir downloads osqueryd. This allows the test
// suite to run on hosts lacking osqueryd. We could consider moving this into a deps step.
func downloadOsqueryInBinDir(binDirectory string) error {
	target := packaging.Target{}
	if err := target.PlatformFromString(runtime.GOOS); err != nil {
		return errors.Wrapf(err, "Error parsing platform: %s", runtime.GOOS)
	}

	outputFile := filepath.Join(binDirectory, "osqueryd") //, target.PlatformBinaryName("osqueryd"))
	cacheDir := "/tmp"

	path, err := packaging.FetchBinary(context.TODO(), cacheDir, "osqueryd", target.PlatformBinaryName("osqueryd"), "stable", target)
	if err != nil {
		return errors.Wrap(err, "An error occurred fetching the osqueryd binary")
	}

	if err := fsutil.CopyFile(path, outputFile); err != nil {
		return errors.Wrapf(err, "Couldn't copy file to %s", outputFile)
	}

	return nil
}

func TestBadBinaryPath(t *testing.T) {
	t.Parallel()
	rootDirectory, rmRootDirectory, err := osqueryTempDir()
	require.NoError(t, err)
	defer rmRootDirectory()

	runner, err := LaunchInstance(
		WithRootDirectory(rootDirectory),
		WithOsquerydBinary("/foobar"),
	)
	assert.Error(t, err)
	assert.Nil(t, runner)
}

func TestWithOsqueryFlags(t *testing.T) {
	t.Parallel()
	rootDirectory, rmRootDirectory, err := osqueryTempDir()
	require.NoError(t, err)
	defer rmRootDirectory()

	runner, err := LaunchInstance(
		WithRootDirectory(rootDirectory),
		WithOsquerydBinary(testOsqueryBinaryDirectory),
		WithOsqueryFlags([]string{"verbose=false"}),
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"verbose=false"}, runner.instance.opts.osqueryFlags)
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

	runner, err := LaunchInstance(
		WithRootDirectory(rootDirectory),
		WithOsquerydBinary(testOsqueryBinaryDirectory),
	)
	require.NoError(t, err)

	waitHealthy(t, runner)

	require.NotEmpty(t, runner.instance.stats.StartTime, "start time should be added to instance stats on start up")
	require.NotEmpty(t, runner.instance.stats.ConnectTime, "connect time should be added to instance stats on start up")

	require.NoError(t, runner.Shutdown())
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

	runner, err := LaunchInstance(
		WithRootDirectory(rootDirectory),
		WithOsquerydBinary(testOsqueryBinaryDirectory),
	)
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

	runner := newRunner(WithRootDirectory(rootDirectory))
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

	extensionSocketPath := filepath.Join(rootDirectory, "sock")
	runner, err := LaunchInstance(
		WithRootDirectory(rootDirectory),
		WithExtensionSocketPath(extensionSocketPath),
		WithOsquerydBinary(testOsqueryBinaryDirectory),
	)
	require.NoError(t, err)

	waitHealthy(t, runner)

	// wait for the launcher-provided extension to register
	time.Sleep(2 * time.Second)

	client, err := osquery.NewClient(extensionSocketPath, 5*time.Second)
	require.NoError(t, err)
	defer client.Close()

	resp, err := client.Query("select * from launcher_gc_info")
	require.NoError(t, err)
	assert.Equal(t, int32(0), resp.Status.Code)
	assert.Equal(t, "OK", resp.Status.Message)
}

// sets up an osquery instance with a running extension to be used in tests.
func setupOsqueryInstanceForTests(t *testing.T) (runner *Runner, teardown func()) {
	rootDirectory, rmRootDirectory, err := osqueryTempDir()
	require.NoError(t, err)

	runner, err = LaunchInstance(
		WithRootDirectory(rootDirectory),
		WithOsquerydBinary(testOsqueryBinaryDirectory),
	)
	require.NoError(t, err)
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
