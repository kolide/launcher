package augeas

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInstallLenses(t *testing.T) {
	t.Parallel()

	tmpDir, err := ioutil.TempDir("", "TestInstallLenses")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	require.NoError(t, InstallLenses(tmpDir), "install lenses")

	files, err := ioutil.ReadDir(tmpDir)
	require.NoError(t, err)

	require.Greater(t, len(files), 5, "Has enough files")

	require.NoError(t, os.Remove(filepath.Join(tmpDir, "pam.aug")))
	require.NoError(t, os.Remove(filepath.Join(tmpDir, "util.aug")))

	// Replace missing files
	require.NoError(t, InstallLenses(tmpDir), "reinstall lenses")
	files2, err := ioutil.ReadDir(tmpDir)
	require.NoError(t, err)

	require.Equal(t, len(files), len(files2))
}
