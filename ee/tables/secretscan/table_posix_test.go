//go:build !windows

package secretscan

import (
	"path/filepath"
	"testing"

	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestScanPathFIFO(t *testing.T) {
	t.Parallel()

	tbl := &Table{
		slogger: multislogger.NewNopLogger(),
	}

	// Initialize config for direct scanPath calls
	cfg, err := newDefaultConfig()
	require.NoError(t, err)
	tbl.defaultConfig = &cfg

	tempDir := t.TempDir()
	fifoPath := filepath.Join(tempDir, "test_fifo")

	err = unix.Mkfifo(fifoPath, 0644)
	require.NoError(t, err, "creating FIFO")

	// Scanning a FIFO should return an error
	_, err = tbl.scanPath(t.Context(), fifoPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported file type")
}
