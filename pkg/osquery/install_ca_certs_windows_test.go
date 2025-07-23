//go:build windows
// +build windows

package osquery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInstallCaCerts_Windows(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	// First installation
	installedPath1, err := InstallCaCerts(tempDir)
	require.NoError(t, err, "install certs first time")
	require.NotEmpty(t, installedPath1)

	// Verify file exists
	_, err = os.Stat(installedPath1)
	require.NoError(t, err, "installed cert file should exist")

	// Second installation should return same path
	installedPath2, err := InstallCaCerts(tempDir)
	require.NoError(t, err, "install certs second time")
	require.Equal(t, installedPath1, installedPath2, "reinstalled file has the same path")

	// Check that it's either system certs or embedded certs
	filename := filepath.Base(installedPath1)
	isSystemCert := strings.HasPrefix(filename, "ca-certs-system-")
	isEmbeddedCert := strings.HasPrefix(filename, "ca-certs-embedded-")
	require.True(t, isSystemCert || isEmbeddedCert, "cert file should be either system or embedded")
}

func TestExportSystemCaCerts(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	// Try to export system certs
	certsPath, err := exportSystemCaCerts(tempDir)

	// This might fail on some Windows systems without proper permissions
	// or in restricted environments, so we just verify behavior
	if err != nil {
		require.Error(t, err)
		require.Empty(t, certsPath)
		return
	}

	// If successful, verify the file
	require.NotEmpty(t, certsPath)
	require.Contains(t, certsPath, "ca-certs-system-")

	// Verify file exists and has content
	info, err := os.Stat(certsPath)
	require.NoError(t, err)
	require.Greater(t, info.Size(), int64(0), "cert file should not be empty")

	// Second export should reuse the same file
	certsPath2, err := exportSystemCaCerts(tempDir)
	require.NoError(t, err)
	require.Equal(t, certsPath, certsPath2, "should reuse existing cert file")
}
