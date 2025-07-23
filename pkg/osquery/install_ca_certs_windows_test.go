//go:build windows
// +build windows

package osquery

import (
	"crypto/x509"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallCaCerts_Windows(t *testing.T) {
	t.Parallel()

	// Create a temporary directory for testing
	dir := t.TempDir()

	// First call should create the file
	caFile, err := InstallCaCerts(dir)
	require.NoError(t, err, "expected no error on first call to InstallCaCerts")
	assert.NotEmpty(t, caFile, "expected a non-empty path for the CA file")

	// Check that the file exists
	_, err = os.Stat(caFile)
	require.NoError(t, err, "expected CA file to exist")

	// Check that the file content matches the embedded bundle
	content, err := os.ReadFile(caFile)
	require.NoError(t, err, "expected to be able to read the CA file")
	assert.Equal(t, defaultCaCerts, content, "expected file content to match embedded CA certs")

	// Second call should not fail
	caFile2, err := InstallCaCerts(dir)
	require.NoError(t, err, "expected no error on second call to InstallCaCerts")
	assert.Equal(t, caFile, caFile2, "expected the same file path on second call")
}

func TestGetSystemCertPool_Windows(t *testing.T) {
	t.Parallel()

	pool, err := GetSystemCertPool()
	require.NoError(t, err, "expected no error when getting system cert pool")
	require.NotNil(t, pool, "expected a non-nil cert pool")

	// To check if the pool is populated, we can compare it to a new empty pool.
	// If they are not equal, it means the system pool has certificates.
	emptyPool := x509.NewCertPool()
	assert.False(t, pool.Equal(emptyPool), "expected the system cert pool to not be empty")
}
