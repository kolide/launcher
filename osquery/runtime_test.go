package osquery

import (
	"context"
	"errors"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kolide/kit/testutil"
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

	paths, err := calculateOsqueryPaths(binDir)
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
	rootDirectory, rmRootDirectory, err := osqueryTempDir()
	require.NoError(t, err)
	defer rmRootDirectory()

	require.NoError(t, buildOsqueryExtensionInBinDir(getBinDir(t)))
	runner, err := LaunchInstance(WithRootDirectory(rootDirectory))
	require.NoError(t, err)

	waitHealthy(t, runner)

	require.NoError(t, runner.Restart())
	waitHealthy(t, runner)

	require.NoError(t, runner.Restart())
	waitHealthy(t, runner)

	require.NoError(t, runner.Shutdown())
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
