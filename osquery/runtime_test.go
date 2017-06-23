package osquery

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCalculateOsqueryPaths(t *testing.T) {
	tempDir := filepath.Dir(os.TempDir())

	// the launcher will verify that the supplied osquery path is an actual file,
	// so we create a mock file on the filesystem to satisfy this requirement
	fakeOsquerydPath := filepath.Join(tempDir, "osqueryd")
	require.NoError(t, ioutil.WriteFile(fakeOsquerydPath, []byte(""), 0755))

	// the launcher expects an osquery extension to be right next to the launcher
	// binary on the filesystem so we doctor os.Args here and create a mock file
	// on the filesystem to satisfy this requirement
	os.Args = []string{fmt.Sprintf("%s/launcher", tempDir)}
	fakeExtensionPath := filepath.Join(tempDir, "osquery-extension.ext")
	require.NoError(t, ioutil.WriteFile(fakeExtensionPath, []byte(""), 0755))

	paths, err := calculateOsqueryPaths(fakeOsquerydPath, tempDir)
	require.NoError(t, err)

	// ensure that the path of the binary is actually what we told the function
	// that it should be
	require.Equal(t, fakeOsquerydPath, paths.BinaryPath)

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

	cmd, err := createOsquerydCommand(paths)
	require.NoError(t, err)
	require.Equal(t, os.Stderr, cmd.Stderr)
	require.Equal(t, os.Stdout, cmd.Stdout)
}

func TestLaunchOsqueryInstance(t *testing.T) {
	if _, err := os.Stat("/usr/bin/osqueryd"); os.IsNotExist(err) {
		t.Fatal("osqueryd not found")
	}
}
