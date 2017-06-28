package osquery

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBestPractices(t *testing.T) {
	rootDirectory, err, rmRootDirectory := osqueryTempDir()
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
	// results is nil
	fmt.Printf("results : < %+v >\n", results)
	// err is "Error running query: no such table: kolide_best_practices"
	fmt.Printf("err : < %+v >\n", err)

	// You can enumerate the whole registry from osquery's perspective. spotlight
	// and kolide_best_practices are not there, but both internal_noop plugins
	// are there (config and logger)
	registry, err := instance.Query("select * from osquery_registry")
	require.NoError(t, err)
	fmt.Println("Printing the osquery registry...")
	for _, row := range registry {
		fmt.Printf("%s\t\t- %s\n", row["registry"], row["name"])
	}

	// now it works. wtf?
	results2, err := instance.Query("select * from kolide_best_practices")
	fmt.Printf("results2 : < %+v >\n", results2)
	fmt.Printf("err : < %+v >\n", err)
	require.NoError(t, err)
	require.Len(t, results2, 1)

	passwordRequiredFromScreensaver, ok := results2[0]["password_required_from_screensaver"]
	require.True(t, ok)
	require.Equal(t, "true", passwordRequiredFromScreensaver)

	require.NoError(t, instance.Kill())
}
