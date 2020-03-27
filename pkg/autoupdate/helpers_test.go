package autoupdate

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckExecutablePermissions(t *testing.T) {
	t.Parallel()

	require.Error(t, checkExecutablePermissions(""), "passing empty string")
	require.Error(t, checkExecutablePermissions("/random/path/should/not/exist"), "passing non-existent file path")

	// Setup the tests
	tmpDir, err := ioutil.TempDir("", "test-autoupdate-check-executable")
	defer os.RemoveAll(tmpDir)
	require.NoError(t, err)

	require.Error(t, checkExecutablePermissions(tmpDir), "directory should not be executable")

	dotExe := ""
	if runtime.GOOS == "windows" {
		dotExe = ".exe"
	}

	fileName := filepath.Join(tmpDir, "file") + dotExe
	tmpFile, err := os.Create(fileName)
	require.NoError(t, err, "os create")
	tmpFile.Close()

	hardLink := filepath.Join(tmpDir, "hardlink") + dotExe
	require.NoError(t, os.Link(fileName, hardLink), "making link")

	symLink := filepath.Join(tmpDir, "symlink") + dotExe
	require.NoError(t, os.Symlink(fileName, symLink), "making symlink")

	// windows doesn't have an executable bit
	if runtime.GOOS == "windows" {
		require.NoError(t, checkExecutablePermissions(fileName), "plain file")
		require.NoError(t, checkExecutablePermissions(hardLink), "hard link")
		require.NoError(t, checkExecutablePermissions(symLink), "symlink")
	} else {
		require.Error(t, checkExecutablePermissions(fileName), "plain file")
		require.Error(t, checkExecutablePermissions(hardLink), "hard link")
		require.Error(t, checkExecutablePermissions(symLink), "symlink")

		require.NoError(t, os.Chmod(fileName, 0755))
		require.NoError(t, checkExecutablePermissions(fileName), "plain file")
		require.NoError(t, checkExecutablePermissions(hardLink), "hard link")
		require.NoError(t, checkExecutablePermissions(symLink), "symlink")
	}
}
