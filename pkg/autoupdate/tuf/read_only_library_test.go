package tuf

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Masterminds/semver"
	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"
)

func Test_newReadOnlyLibrary(t *testing.T) {
	t.Parallel()

	_, err := newReadOnlyLibrary("/some/path/to/a/fake/directory", log.NewNopLogger())
	require.Error(t, err, "expected error when creating library with nonexistent base dir")

	testBaseDir := filepath.Join(t.TempDir(), "updates")
	_, err = newReadOnlyLibrary(testBaseDir, log.NewNopLogger())
	require.Error(t, err, "expected error when creating library with nonexistent libraries")

	require.NoError(t, os.MkdirAll(filepath.Join(testBaseDir, "launcher"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(testBaseDir, "osqueryd"), 0755))
	_, err = newReadOnlyLibrary(testBaseDir, log.NewNopLogger())
	require.NoError(t, err, "expected no error when creating library")
}

func TestMostRecentVersion(t *testing.T) {
	t.Parallel()

	t.Skip("TODO")
}

func TestPathToTargetVersionExecutable(t *testing.T) {
	t.Parallel()

	t.Skip("TODO")
}

func TestAvailable(t *testing.T) {
	t.Parallel()

	// Create update directories
	testBaseDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(testBaseDir, "launcher"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(testBaseDir, "osqueryd"), 0755))

	// Set up test library
	testReadOnlyLibrary, err := newReadOnlyLibrary(testBaseDir, log.NewNopLogger())
	require.NoError(t, err, "unexpected error creating new read-only library")

	// Set up valid "osquery" executable
	runningOsqueryVersion := "5.5.7"
	runningTarget := fmt.Sprintf("osqueryd-%s.tar.gz", runningOsqueryVersion)
	executablePath := testReadOnlyLibrary.PathToTargetVersionExecutable(binaryOsqueryd, runningTarget)
	copyBinary(t, executablePath)
	require.NoError(t, os.Chmod(executablePath, 0755))

	// Query for the current osquery version
	require.True(t, testReadOnlyLibrary.Available(binaryOsqueryd, runningTarget))

	// Query for a different osqueryd version
	require.False(t, testReadOnlyLibrary.Available(binaryOsqueryd, "osqueryd-5.6.7.tar.gz"))
}

func Test_installedVersion_cached(t *testing.T) {
	t.Parallel()

	// Create update directories
	testBaseDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(testBaseDir, "launcher"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(testBaseDir, "osqueryd"), 0755))

	// Set up test library
	testReadOnlyLibrary, err := newReadOnlyLibrary(testBaseDir, log.NewNopLogger())
	require.NoError(t, err, "unexpected error creating new read-only library")

	// Create cached version file
	expectedVersion := "5.5.5"
	require.NoError(t, os.WriteFile(filepath.Join(testReadOnlyLibrary.baseDir, "osqueryd-installed-version"), []byte(expectedVersion), 0755))

	// Create fake executable in current working directory
	executablePath, err := os.Executable()
	require.NoError(t, err)
	executableName := "osqueryd"
	if runtime.GOOS == "windows" {
		executableName += ".exe"
	}
	testExecutablePath := filepath.Join(filepath.Dir(executablePath), executableName)
	require.NoError(t, os.WriteFile(testExecutablePath, []byte("test"), 0755))
	t.Cleanup(func() {
		os.Remove(testExecutablePath)
	})

	actualVersion, actualPath, err := testReadOnlyLibrary.installedVersion(binaryOsqueryd)
	require.NoError(t, err, "could not get installed version")
	require.Equal(t, expectedVersion, actualVersion.Original(), "version mismatch")
	require.Equal(t, testExecutablePath, actualPath)
}

func Test_cacheInstalledVersion(t *testing.T) {
	t.Parallel()

	// Create update directories
	testBaseDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(testBaseDir, "launcher"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(testBaseDir, "osqueryd"), 0755))

	// Set up test library
	testReadOnlyLibrary, err := newReadOnlyLibrary(testBaseDir, log.NewNopLogger())
	require.NoError(t, err, "unexpected error creating new read-only library")

	versionToCache, err := semver.NewVersion("1.2.3-45-abcdabcd")
	require.NoError(t, err, "unexpected error parsing semver")

	for _, binary := range binaries {
		// Confirm cache file doesn't exist yet
		expectedCacheFileLocation := testReadOnlyLibrary.cachedInstalledVersionLocation(binary)
		_, err = os.Stat(expectedCacheFileLocation)
		require.True(t, os.IsNotExist(err), "cache file exists but should not have been created yet")

		// Cache it
		testReadOnlyLibrary.cacheInstalledVersion(binary, versionToCache)

		// Confirm cache file exists
		_, err = os.Stat(expectedCacheFileLocation)
		require.NoError(t, err, "cache file %s does not exist but should have been created", expectedCacheFileLocation)

		// Compare versions
		cachedVersion, err := testReadOnlyLibrary.getCachedInstalledVersion(binary)
		require.NoError(t, err, "expected no error reading cached installed version")
		require.True(t, versionToCache.Equal(cachedVersion), "versions do not match")
	}
}

func Test_versionFromTarget(t *testing.T) {
	t.Parallel()

	testVersions := []struct {
		target          string
		binary          autoupdatableBinary
		operatingSystem string
		version         string
	}{
		{
			target:          "launcher/darwin/launcher-0.10.1.tar.gz",
			binary:          binaryLauncher,
			operatingSystem: "darwin",
			version:         "0.10.1",
		},
		{
			target:          "launcher/windows/launcher-1.13.5.tar.gz",
			binary:          binaryLauncher,
			operatingSystem: "windows",
			version:         "1.13.5",
		},
		{
			target:          "launcher/linux/launcher-0.13.5-40-gefdc582.tar.gz",
			binary:          binaryLauncher,
			operatingSystem: "linux",
			version:         "0.13.5-40-gefdc582",
		},
		{
			target:          "osqueryd/darwin/osqueryd-5.8.1.tar.gz",
			binary:          binaryOsqueryd,
			operatingSystem: "darwin",
			version:         "5.8.1",
		},
		{
			target:          "osqueryd/windows/osqueryd-0.8.1.tar.gz",
			binary:          binaryOsqueryd,
			operatingSystem: "windows",
			version:         "0.8.1",
		},
		{
			target:          "osqueryd/linux/osqueryd-5.8.2.tar.gz",
			binary:          binaryOsqueryd,
			operatingSystem: "linux",
			version:         "5.8.2",
		},
	}

	for _, testVersion := range testVersions {
		require.Equal(t, testVersion.version, versionFromTarget(testVersion.binary, filepath.Base(testVersion.target)))
	}
}
