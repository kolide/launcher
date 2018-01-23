package osquery

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/kolide/kit/testutil"
	osquery "github.com/kolide/osquery-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getBinDir finds the directory of the currently running binary (where we will
// look for the osquery extension)
func getBinDir(t *testing.T) string {
	binPath, err := os.Executable()
	require.NoError(t, err)
	binDir := filepath.Dir(binPath)
	return binDir
}

func TestCalculateOsqueryPaths(t *testing.T) {
	t.Parallel()
	binDir := getBinDir(t)
	fakeExtensionPath := filepath.Join(binDir, "osquery-extension.ext")
	require.NoError(t, ioutil.WriteFile(fakeExtensionPath, []byte("#!/bin/bash\nsleep infinity"), 0755))

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

	osquerydPath, err := exec.LookPath("osqueryd")
	require.NoError(t, err)

	cmd, err := createOsquerydCommand(osquerydPath, paths, "config_plugin", "logger_plugin", "distributed_plugin", os.Stdout, os.Stderr)
	require.NoError(t, err)
	require.Equal(t, os.Stderr, cmd.Stderr)
	require.Equal(t, os.Stdout, cmd.Stdout)
}

// buildOsqueryExtensionInBinDir compiles the osquery extension and places it
// on disk in the same directory as the currently running executable (as
// expected when running an osquery process)
func buildOsqueryExtensionInBinDir(rootDirectory string) error {
	goBinary, err := exec.LookPath("go")
	if err != nil {
		return err
	}

	goPath := os.Getenv("GOPATH")
	if goPath == "" {
		goPath = filepath.Join(os.Getenv("HOME"), "go")
		if stat, err := os.Stat(goPath); err != nil || !stat.IsDir() {
			return errors.New("GOPATH is not set and default doesn't exist")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(
		ctx,
		goBinary,
		"build",
		"-o",
		filepath.Join(rootDirectory, "osquery-extension.ext"),
		filepath.Join(goPath, "src/github.com/kolide/launcher/cmd/osquery-extension/osquery-extension.go"),
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

	require.NoError(t, buildOsqueryExtensionInBinDir(getBinDir(t)))
	runner, err := LaunchInstance(
		WithRootDirectory(rootDirectory),
		WithOsquerydBinary("/foobar"),
	)
	assert.Error(t, err)
	assert.Nil(t, runner)
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

	require.NoError(t, buildOsqueryExtensionInBinDir(getBinDir(t)))
	runner, err := LaunchInstance(WithRootDirectory(rootDirectory))
	require.NoError(t, err)

	waitHealthy(t, runner)

	require.NoError(t, runner.Shutdown())
}

func TestRestart(t *testing.T) {
	t.Parallel()
	runner, _, teardown := setupOsqueryInstanceForTests(t)
	defer teardown()

	require.NoError(t, runner.Restart())
	waitHealthy(t, runner)

	require.NoError(t, runner.Restart())
	waitHealthy(t, runner)
}

func TestOsqueryDies(t *testing.T) {
	t.Parallel()
	rootDirectory, rmRootDirectory, err := osqueryTempDir()
	require.NoError(t, err)
	defer rmRootDirectory()

	require.NoError(t, buildOsqueryExtensionInBinDir(getBinDir(t)))
	runner, err := LaunchInstance(WithRootDirectory(rootDirectory))
	require.NoError(t, err)

	waitHealthy(t, runner)

	// Simulate the osquery process unexpectedly dying
	runner.instanceLock.Lock()
	require.NoError(t, killProcessGroup(runner.instance.cmd))
	runner.instance.errgroup.Wait()
	runner.instanceLock.Unlock()

	waitHealthy(t, runner)

	require.NoError(t, runner.Shutdown())
}

func TestNotStarted(t *testing.T) {
	t.Parallel()
	rootDirectory, rmRootDirectory, err := osqueryTempDir()
	require.NoError(t, err)
	defer rmRootDirectory()

	require.NoError(t, buildOsqueryExtensionInBinDir(getBinDir(t)))
	runner := newRunner(WithRootDirectory(rootDirectory))
	require.NoError(t, err)

	assert.Error(t, runner.Healthy())
	assert.NoError(t, runner.Shutdown())
}

func TestExtensionIsCleanedUp(t *testing.T) {
	t.Parallel()

	runner, extensionPid, teardown := setupOsqueryInstanceForTests(t)
	defer teardown()

	osqueryPID := runner.instance.cmd.Process.Pid

	pgid, err := syscall.Getpgid(osqueryPID)
	require.NoError(t, err)
	require.Equal(t, pgid, osqueryPID, "pgid must be set")

	// kill the current osquery process but not the extension
	err = syscall.Kill(osqueryPID, syscall.SIGKILL)
	require.NoError(t, err)

	waitHealthy(t, runner)

	// check that the extension process is no longer running
	extpgid, err := syscall.Getpgid(extensionPid)
	require.EqualError(t, err, "no such process")
	require.Equal(t, extpgid, -1)
}

func TestExtensionSocketPath(t *testing.T) {
	t.Parallel()

	rootDirectory, rmRootDirectory, err := osqueryTempDir()
	require.NoError(t, err)
	defer rmRootDirectory()

	require.NoError(t, buildOsqueryExtensionInBinDir(getBinDir(t)))
	extensionSocketPath := filepath.Join(rootDirectory, "sock")
	runner, err := LaunchInstance(WithRootDirectory(rootDirectory), WithExtensionSocketPath(extensionSocketPath))
	require.NoError(t, err)

	waitHealthy(t, runner)

	// wait for the launcher-provided extension to register
	time.Sleep(2 * time.Second)

	client, err := osquery.NewClient(extensionSocketPath, 5*time.Second)
	require.NoError(t, err)
	defer client.Close()

	resp, err := client.Query("select * from kolide_best_practices")
	require.NoError(t, err)
	assert.Equal(t, int32(0), resp.Status.Code)
	assert.Equal(t, "OK", resp.Status.Message)
}

// sets up an osquery instance with a running extension to be used in tests.
func setupOsqueryInstanceForTests(t *testing.T) (runner *Runner, extensionPid int, teardown func()) {
	rootDirectory, rmRootDirectory, err := osqueryTempDir()
	require.NoError(t, err)

	require.NoError(t, buildOsqueryExtensionInBinDir(getBinDir(t)))
	runner, err = LaunchInstance(WithRootDirectory(rootDirectory))
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
