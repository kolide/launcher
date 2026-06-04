//go:build darwin || linux

package permissions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPermissions confirms that the socket file is created with appropriately-restricted permissions.
func TestPermissions(t *testing.T) {
	t.Parallel()

	// Set up dependencies
	rootDir := t.TempDir()
	testFile := filepath.Join(rootDir, "test.txt")
	fh, err := os.Create(testFile)
	require.NoError(t, err)
	require.NoError(t, fh.Close())

	require.NoError(t, RestrictFileAccessToRootOnly(testFile))

	fi, err := os.Stat(testFile)
	require.NoError(t, err)
	require.Equal(t, "-rw-------", fi.Mode().String()) // 0600
}
