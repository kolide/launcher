//go:build windows

package osquery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func TestInstallCaCerts_Windows(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	slogger := multislogger.NewNopLogger()

	// First installation
	installedPath1, err := InstallCaCerts(tempDir, slogger)
	require.NoError(t, err, "install certs first time")
	require.NotEmpty(t, installedPath1)

	// Verify file exists
	_, err = os.Stat(installedPath1)
	require.NoError(t, err, "installed cert file should exist")

	// Second installation should return same path
	installedPath2, err := InstallCaCerts(tempDir, slogger)
	require.NoError(t, err, "install certs second time")
	require.Equal(t, installedPath1, installedPath2, "reinstalled file has the same path")

	// Check that it's either system certs or embedded certs
	filename := filepath.Base(installedPath1)
	isSystemCert := strings.HasPrefix(filename, "ca-certs-system-")
	isEmbeddedCert := strings.HasPrefix(filename, "ca-certs-embedded-")
	require.True(t, isSystemCert || isEmbeddedCert, "cert file should be either system or embedded")
}

func Test_readSystemCaCerts(t *testing.T) {
	t.Parallel()

	pemData, err := readSystemCaCerts(multislogger.NewNopLogger())
	require.NoError(t, err)
	require.NotEmpty(t, parseCerts(pemData), "system store should contain CA certs")
}
