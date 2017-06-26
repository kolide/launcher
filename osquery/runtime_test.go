package osquery

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCalculateOsqueryPaths(t *testing.T) {
	rootDirectory := prepareExtensionEnvironment(t)
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

// prepareExtensionEnvironment is a helper which prepares the filesystem and
// execution environment so that an osquery instance can be launched in tests.
// The path to the necessary root directory is returned.
func prepareExtensionEnvironment(t *testing.T) string {
	tempDir := filepath.Dir(os.TempDir())

	// the launcher expects an osquery extension to be right next to the launcher
	// binary on the filesystem so we doctor os.Args here and create a mock file
	// on the filesystem to satisfy this requirement
	os.Args = []string{fmt.Sprintf("%s/launcher", tempDir)}
	fakeExtensionPath := filepath.Join(tempDir, "osquery-extension.ext")
	require.NoError(t, ioutil.WriteFile(fakeExtensionPath, []byte("#!/bin/bash\nsleep infinity"), 0755))

	return tempDir
}

func TestOsqueryRuntime(t *testing.T) {
	rootDirectory := prepareExtensionEnvironment(t)
	instance, err := LaunchOsqueryInstance(WithRootDirectory(rootDirectory))
	require.NoError(t, err)

	healthy, err := instance.Healthy()
	require.NoError(t, err)
	require.True(t, healthy)

	require.NoError(t, instance.Kill())
}
