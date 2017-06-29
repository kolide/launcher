package osquery

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBestPractices(t *testing.T) {
	rootDirectory, rmRootDirectory, err := osqueryTempDir()
	require.NoError(t, err)
	defer rmRootDirectory()

	// this could be `defer falsifyOsArgs(rootDirectory)()` but this may be more clear
	cancelFunc := falsifyOsArgs(rootDirectory)
	defer cancelFunc()

	require.NoError(t, buildOsqueryExtensionInTempDir(rootDirectory))
	instance, err := LaunchOsqueryInstance(WithRootDirectory(rootDirectory))
	require.NoError(t, err)

	healthy, err := instance.Healthy()
	require.NoError(t, err)
	require.True(t, healthy)

	results, err := instance.Query("select * from kolide_best_practices")
	require.NoError(t, err)
	require.Len(t, results, 1)

	passwordRequiredFromScreensaver, ok := results[0]["password_required_from_screensaver"]
	require.True(t, ok)
	require.Equal(t, "true", passwordRequiredFromScreensaver)

	require.NoError(t, instance.Kill())
}
