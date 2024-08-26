//go:build !windows
// +build !windows

package launcher

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDetermineRootDirectoryOverride(t *testing.T) {
	t.Parallel()

	// On non-Windows OSes, we don't override the root directory -- confirm we always return
	// optsRootDir instead of an override
	optsRootDir := filepath.Join("some", "dir", "somewhere")
	require.Equal(t, optsRootDir, DetermineRootDirectoryOverride(optsRootDir, "", ""))
}
