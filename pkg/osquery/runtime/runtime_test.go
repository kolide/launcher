//go:build !windows
// +build !windows

package runtime

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kolide/kit/fs"
	"github.com/kolide/kit/testutil"
	"github.com/kolide/launcher/pkg/packaging"
	ps "github.com/mitchellh/go-ps"
	osquery "github.com/osquery/osquery-go"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
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
	testOsqueryBinaryDirectory = filepath.Join(binDirectory, "osqueryd")

	if err := downloadOsqueryInBinDir(binDirectory); err != nil {
		fmt.Printf("Failed to download osquery: %v\n", err)
		os.Exit(1)
	}

	// Build the osquerty extension once
	binDir, err := getBinDir()
	if err != nil {
		fmt.Printf("Failed to get binDir: %v\n", err)
		os.Exit(1)
	}

	if err := buildOsqueryExtensionInBinDir(binDir); err != nil {
		fmt.Printf("Failed to build osquery extension: %v\n", err)
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

	paths, err := calculateOsqueryPaths(binDir, "")
	require.NoError(t, err)

	// ensure that all of our resulting artifact files are in the rootDir that we
	// dictated
	require.Equal(t, binDir, filepath.Dir(paths.pidfilePath))
	require.Equal(t, binDir, filepath.Dir(paths.databasePath))
	require.Equal(t, binDir, filepath.Dir(paths.extensionPath))
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
	// Ensure that the provided flags were placed last (so that they can override)
	assert.Equal(
		t,
		[]string{"--verbose=false", "--windows_event_channels=foo,bar"},
		// 7 is for the 7 flags that cannot be overridden with this option.
		cmd.Args[len(cmd.Args)-2-7:len(cmd.Args)-7],
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

	if err := fs.CopyFile(path, outputFile); err != nil {
		return errors.Wrapf(err, "Couldn't copy file to %s", outputFile)
	}

	return nil
}

// buildOsqueryExtensionInBinDir compiles the osquery extension and places it
// on disk in the same directory as the currently running executable (as
// expected when running an osquery process)
func buildOsqueryExtensionInBinDir(rootDirectory string) error {
	goBinary, err := exec.LookPath("go")
	if err != nil {
		return err
	}

	_, myFilename, _, _ := runtime.Caller(1)
	launcherSrcDir := filepath.Join(filepath.Dir(myFilename), "..", "..", "..")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(
		ctx,
		goBinary,
		"build",
		"-o",
		filepath.Join(rootDirectory, "osquery-extension.ext"),
		filepath.Join(launcherSrcDir, "cmd/osquery-extension/osquery-extension.go"),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
	runner, _, teardown := setupOsqueryInstanceForTests(t)
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
	require.NotEmpty(t, previousStats.ExitTime, "exit time should be added to instance when unexpedted shutdown")

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

	runner, extensionPid, teardown := setupOsqueryInstanceForTests(t)
	defer teardown()

	osqueryPID := runner.instance.cmd.Process.Pid

	pgid, err := syscall.Getpgid(osqueryPID)
	require.NoError(t, err)
	require.Equal(t, pgid, osqueryPID, "pgid must be set")

	extensionProcess, err := ps.FindProcess(extensionPid)
	require.NoError(t, err)
	// process name seems truncated
	require.True(t, strings.HasPrefix(extensionProcess.Executable(), "osquery-ext"))

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

	// check that the extension process is no longer running. Because
	// this may be subject to PID reuse as a false positive, we have two
	// test patterns.  If we got an err, then the process is probably
	// gone, and we test one way. If err==nil, check for PID
	// reuse. go-ps will panic if you look for a missing process, so
	// there is still some window for errors.
	extpgid, err := syscall.Getpgid(extensionPid)
	if err != nil {
		require.EqualError(t, err, "no such process")
		require.Equal(t, extpgid, -1)
	} else {
		extensionProcess, err := ps.FindProcess(extensionPid)
		require.NoError(t, err)
		require.False(t, strings.HasPrefix(extensionProcess.Executable(), "osquery-ext"), "old extension pid, still running. And named like osquery-extension")
		require.NotEqual(t, osqueryPID, extpgid)
	}

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
func setupOsqueryInstanceForTests(t *testing.T) (runner *Runner, extensionPid int, teardown func()) {
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

	extensionPid = getExtensionPid(t, rootDirectory, pgid)

	teardown = func() {
		defer rmRootDirectory()
		require.NoError(t, runner.Shutdown())
	}
	return runner, extensionPid, teardown
}

// get the osquery-extension.ext process' PID
func getExtensionPid(t *testing.T, rootDirectory string, pgid int) int {
	out, err := exec.Command("ps", "xao", "pid,ppid,pgid,comm").CombinedOutput()
	require.NoError(t, err)

	var extensionPid int
	r := bufio.NewReader(bytes.NewReader(out))
	for {
		line, _, err := r.ReadLine()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)

		// some versions of the ps command truncate the comm field, using osquery-extensi
		// instead of the full command name.
		if bytes.Contains(line, []byte(`osquery-extensi`)) &&
			bytes.Contains(line, []byte(fmt.Sprintf("%d", pgid))) {
			line = bytes.TrimSpace(line)

			cols := bytes.Split(line, []byte(" "))
			require.NotEqual(t, len(cols), 0)

			pid, err := strconv.Atoi(string(cols[0]))
			require.NoError(t, err)
			extensionPid = pid
		}
	}

	require.NotZero(t, extensionPid)
	return extensionPid
}
