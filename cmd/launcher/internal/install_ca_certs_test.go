package internal

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestInstallCaCerts is a very quick test, just that the file will be
// written correctly.
func TestInstallCaCerts(t *testing.T) {
	t.Parallel()

	tempDir, err := os.MkdirTemp("", "test-install-certs")
	require.NoError(t, err, "mktemp dir")
	defer os.RemoveAll(tempDir)

	var writtenFiles []string

	installedPath1, err := InstallCaCerts(tempDir)
	require.NoError(t, err, "install certs one")

	installedPath2, err := InstallCaCerts(tempDir)
	require.NoError(t, err, "install certs two")

	require.Equal(t, installedPath1, installedPath2, "reinstalled file has the same path")

	f1Eq, err := filesEqual(installedPath1, "install_ca_certs.go")
	require.NoError(t, err, "comparing f1")
	require.True(t, f1Eq, "f1 contents")

	f2Eq, err := filesEqual(installedPath2, "install_ca_certs.go")
	require.NoError(t, err, "comparing f2")
	require.True(t, f2Eq, "f2 contents")
}

func filesEqual(f1, f2 string) (bool, error) {
	f1Bytes, err := os.ReadFile(f2)
	if err != nil {
		return false, fmt.Errorf("reading f1 (%s): %w", f1, err)
	}

	f2Bytes, err := os.ReadFile(f2)
	if err != nil {
		return false, fmt.Errorf("reading f2 (%s): %w", f2, err)
	}

	return bytes.Equal(f1Bytes, f2Bytes), nil
}
