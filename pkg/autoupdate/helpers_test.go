package autoupdate

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckExecutable(t *testing.T) {
	t.Parallel()

	require.Error(t, checkExecutable(""), "passing empty string")
	require.Error(t, checkExecutable("/random/path/should/not/exist"), "passing empty string")

	// Setup the tests
	tmpDir, err := ioutil.TempDir("", "test-autoupdate-check-executable")
	defer os.RemoveAll(tmpDir)
	require.NoError(t, err)

	require.Error(t, checkExecutable(tmpDir), "directory should not be executable")

	fileName := filepath.Join(tmpDir, "file")
	tmpFile, err := os.Create(fileName)
	require.NoError(t, err, "os create")
	tmpFile.Close()

	hardLink := filepath.Join(tmpDir, "hardlink")
	require.NoError(t, os.Link(fileName, hardLink), "making link")

	symLink := filepath.Join(tmpDir, "symlink")
	require.NoError(t, os.Symlink(fileName, symLink), "making symlink")

	require.Error(t, checkExecutable(fileName), "plain file")
	require.Error(t, checkExecutable(hardLink), "hard link")
	require.Error(t, checkExecutable(symLink), "symlink")

	require.NoError(t, os.Chmod(fileName, 0755))
	require.NoError(t, checkExecutable(fileName), "plain file")
	require.NoError(t, checkExecutable(hardLink), "hard link")
	require.NoError(t, checkExecutable(symLink), "symlink")
}
