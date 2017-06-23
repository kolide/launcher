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

	"github.com/kolide/osquery-go/plugin/config"
	"github.com/kolide/osquery-go/plugin/logger"
	"github.com/stretchr/testify/require"
)

func findOsquerydBinaryPath(t *testing.T) string {
	path, err := exec.LookPath("osqueryd")
	require.NoError(t, err)
	_, err = os.Stat(path)
	require.NoError(t, err)
	return path
}

func TestCalculateOsqueryPaths(t *testing.T) {
	tempDir := filepath.Dir(os.TempDir())

	osquerydPath := findOsquerydBinaryPath(t)

	// the launcher expects an osquery extension to be right next to the launcher
	// binary on the filesystem so we doctor os.Args here and create a mock file
	// on the filesystem to satisfy this requirement
	os.Args = []string{fmt.Sprintf("%s/launcher", tempDir)}
	fakeExtensionPath := filepath.Join(tempDir, "osquery-extension.ext")
	require.NoError(t, ioutil.WriteFile(fakeExtensionPath, []byte("#!/bin/bash\nsleep infinity"), 0755))

	paths, err := calculateOsqueryPaths(osquerydPath, tempDir)
	require.NoError(t, err)

	// ensure that the path of the binary is actually what we told the function
	// that it should be
	require.Equal(t, osquerydPath, paths.BinaryPath)

	// ensure that all of our resulting artifact files are in the rootDir that we
	// dictated
	require.Equal(t, tempDir, filepath.Dir(paths.PidfilePath))
	require.Equal(t, tempDir, filepath.Dir(paths.DatabasePath))
	require.Equal(t, tempDir, filepath.Dir(paths.ExtensionPath))
	require.Equal(t, tempDir, filepath.Dir(paths.ExtensionSocketPath))
	require.Equal(t, tempDir, filepath.Dir(paths.ExtensionAutoloadPath))
}

func TestCreateOsqueryCommand(t *testing.T) {
	paths := &osqueryFilePaths{
		PidfilePath:           "/foo/bar/osquery.pid",
		DatabasePath:          "/foo/bar/osquery.db",
		ExtensionSocketPath:   "/foo/bar/osquery.sock",
		ExtensionAutoloadPath: "/foo/bar/osquery.autoload",
	}

	cmd, err := createOsquerydCommand(paths, "config_plugin", "logger_plugin")
	require.NoError(t, err)
	require.Equal(t, os.Stderr, cmd.Stderr)
	require.Equal(t, os.Stdout, cmd.Stdout)
}

func TestOsqueryRuntime(t *testing.T) {
	osquerydPath := findOsquerydBinaryPath(t)
	tempDir := filepath.Dir(os.TempDir())

	// the launcher expects an osquery extension to be right next to the launcher
	// binary on the filesystem so we doctor os.Args here and create a mock file
	// on the filesystem to satisfy this requirement
	os.Args = []string{fmt.Sprintf("%s/launcher", tempDir)}
	fakeExtensionPath := filepath.Join(tempDir, "osquery-extension.ext")
	require.NoError(t, ioutil.WriteFile(fakeExtensionPath, []byte("#!/bin/bash\nsleep infinity"), 0755))

	generateConfigs := func(ctx context.Context) (map[string]string, error) {
		t.Log("osquery config requested")
		return map[string]string{}, nil
	}

	logString := func(ctx context.Context, typ logger.LogType, logText string) error {
		t.Logf("%s: %s\n", typ, logText)
		return nil
	}

	instance, err := LaunchOsqueryInstance(
		osquerydPath,
		tempDir,
		"foo",
		"bar",
		WithPlugin(config.NewPlugin("foo", generateConfigs)),
		WithPlugin(logger.NewPlugin("bar", logString)),
	)
	require.NoError(t, err)

	// Give osquery some time to boot, start the plugins, and execute for a bit
	time.Sleep(10 * time.Second)

	healthy, err := instance.Healthy()
	require.NoError(t, err)
	require.True(t, healthy)

	require.NoError(t, instance.Kill())
}
