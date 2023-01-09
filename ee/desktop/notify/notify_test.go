package notify

import (
	"strings"
	"testing"

	"github.com/kolide/launcher/ee/desktop/assets"
	"github.com/stretchr/testify/require"
)

func Test_setIconPath(t *testing.T) {
	t.Parallel()

	// Create a temp directory to use as our root directory
	rootDir := t.TempDir()

	// Test that if the icon doesn't exist in the root dir, the notifier will create it.
	iconPath, err := setIconPath(rootDir)
	require.NoError(t, err, "expected no error when setting icon path")
	require.True(t, strings.HasPrefix(iconPath, rootDir), "unexpected location for icon")
	require.True(t, strings.HasSuffix(iconPath, assets.KolideIconFilename), "unexpected file name for icon")

	// Test that if the icon already exists, the notifier will return the correct location.
	preexistingIconPath, err := setIconPath(rootDir)
	require.NoError(t, err, "expected no error when setting icon path")
	require.True(t, strings.HasPrefix(preexistingIconPath, rootDir), "unexpected location for icon")
	require.True(t, strings.HasSuffix(preexistingIconPath, assets.KolideIconFilename), "unexpected file name for icon")
}
