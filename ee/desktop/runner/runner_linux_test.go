//go:build linux

package runner

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNixXdgDataDirs(t *testing.T) {
	t.Parallel()

	username := "testuser"
	result := nixXdgDataDirs(username)

	// Validate that the result is a well-formed string suitable for XDG_DATA_DIRS
	require.NotEmpty(t, result, "should not be empty")
	require.NotContains(t, result, "::", "should not contain double colons")
	require.False(t, strings.HasPrefix(result, ":"), "should not start with colon")
	require.False(t, strings.HasSuffix(result, ":"), "should not end with colon")

	// Validate that all paths are absolute and non-empty
	parts := strings.Split(result, ":")
	require.Greater(t, len(parts), 0, "XDG_DATA_DIRS should contain at least one path")
	for i, part := range parts {
		require.NotEmpty(t, part, "path %d should not be empty", i)
		require.True(t, strings.HasPrefix(part, "/"), "path %d should be absolute (start with /)", i)
	}

	// Confirm we included our hardcoded paths
	require.Contains(t, result, "/nix/profile/share")
	require.Contains(t, result, "/nix/var/nix/profiles/default/share")
	require.Contains(t, result, "/run/current-system/sw/share")

	// Confirm we included our username-based paths
	require.Contains(t, result, "/home/testuser/.nix-profile/share")
	require.Contains(t, result, "/home/testuser/.local/state/nix/profile/share")
	require.Contains(t, result, "/etc/profiles/per-user/testuser/share")
}
