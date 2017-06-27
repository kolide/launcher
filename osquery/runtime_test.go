package osquery

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCalculateOsqueryPaths(t *testing.T) {
	rootDirectory := filepath.Dir(os.TempDir())

	// the launcher expects an osquery extension to be right next to the launcher
	// binary on the filesystem so we doctor os.Args here and create a mock file
	// on the filesystem to satisfy this requirement
	previousArgs := os.Args
	os.Args = []string{fmt.Sprintf("%s/launcher", rootDirectory)}
	defer func() {
		os.Args = previousArgs
	}()

	fakeExtensionPath := filepath.Join(rootDirectory, "osquery-extension.ext")
	require.NoError(t, ioutil.WriteFile(fakeExtensionPath, []byte("#!/bin/bash\nsleep infinity"), 0755))

	paths, err := calculateOsqueryPaths(rootDirectory)
	require.NoError(t, err)

	// ensure that all of our resulting artifact files are in the rootDir that we
	// dictated
	require.Equal(t, rootDirectory, filepath.Dir(paths.pidfilePath))
	require.Equal(t, rootDirectory, filepath.Dir(paths.databasePath))
	require.Equal(t, rootDirectory, filepath.Dir(paths.extensionPath))
	require.Equal(t, rootDirectory, filepath.Dir(paths.extensionSocketPath))
	require.Equal(t, rootDirectory, filepath.Dir(paths.extensionAutoloadPath))
}

func TestCreateOsqueryCommand(t *testing.T) {
	paths := &osqueryFilePaths{
		pidfilePath:           "/foo/bar/osquery.pid",
		databasePath:          "/foo/bar/osquery.db",
		extensionSocketPath:   "/foo/bar/osquery.sock",
		extensionAutoloadPath: "/foo/bar/osquery.autoload",
	}

	osquerydPath, err := exec.LookPath("osqueryd")
	require.NoError(t, err)

	cmd, err := createOsquerydCommand(osquerydPath, paths, "config_plugin", "logger_plugin")
	require.NoError(t, err)
	require.Equal(t, os.Stderr, cmd.Stderr)
	require.Equal(t, os.Stdout, cmd.Stdout)
}

func TestOsqueryRuntime(t *testing.T) {
	rootDirectory := filepath.Dir(os.TempDir())

	// the launcher expects an osquery extension to be right next to the launcher
	// binary on the filesystem so we doctor os.Args here and create a mock file
	// on the filesystem to satisfy this requirement
	previousArgs := os.Args
	os.Args = []string{fmt.Sprintf("%s/launcher", rootDirectory)}
	defer func() {
		os.Args = previousArgs
	}()

	// Drop the actual version of our extension on disk so that we can get as
	// realistic of a test environment as possible
	goBinary, err := exec.LookPath("go")
	require.NoError(t, err)
	goPath := os.Getenv("GOPATH")
	require.NotEmpty(t, goPath)
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
	require.NoError(t, cmd.Run())

	instance, err := LaunchOsqueryInstance(WithRootDirectory(rootDirectory))
	require.NoError(t, err)

	// Sleep to avoid race condition: https://issues.apache.org/jira/browse/THRIFT-4237
	time.Sleep(1 * time.Second)

	healthy, err := instance.Healthy()
	require.NoError(t, err)
	require.True(t, healthy)

	require.NoError(t, instance.Kill())
}
