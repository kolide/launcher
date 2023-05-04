package tuf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Masterminds/semver"
	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/autoupdate"
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

func TestPathToTargetVersionExecutable(t *testing.T) {
	t.Parallel()

	testBaseDir := filepath.Join(t.TempDir(), "updates")
	require.NoError(t, os.MkdirAll(filepath.Join(testBaseDir, "launcher"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(testBaseDir, "osqueryd"), 0755))
	testLibrary, err := newReadOnlyLibrary(testBaseDir, log.NewNopLogger())
	require.NoError(t, err, "expected no error when creating library")

	testVersion := "1.0.7-30-abcdabcd"
	testTargetFilename := fmt.Sprintf("launcher-%s.tar.gz", testVersion)
	expectedPath := filepath.Join(testBaseDir, "launcher", testVersion, "launcher")
	if runtime.GOOS == "darwin" {
		expectedPath = filepath.Join(testBaseDir, "launcher", testVersion, "Kolide.app", "Contents", "MacOS", "launcher")
	} else if runtime.GOOS == "windows" {
		expectedPath = expectedPath + ".exe"
	}

	actualPath := testLibrary.PathToTargetVersionExecutable(binaryLauncher, testTargetFilename)
	require.Equal(t, expectedPath, actualPath, "path mismatch")
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

func Test_sortedVersionsInLibrary(t *testing.T) {
	t.Parallel()

	// Create update directories
	testBaseDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(testBaseDir, "launcher"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(testBaseDir, "osqueryd"), 0755))

	// Create an update in the library that is invalid because it doesn't have a valid version
	invalidVersion := "not_a_semver_1"
	require.NoError(t, os.MkdirAll(filepath.Join(testBaseDir, "launcher", invalidVersion), 0755))

	// Create an update in the library that is invalid because it's corrupted
	corruptedVersion := "1.0.6-11-abcdabcd"
	corruptedVersionDirectory := filepath.Join(testBaseDir, "launcher", corruptedVersion)
	corruptedVersionLocation := executableLocation(corruptedVersionDirectory, binaryLauncher)
	require.NoError(t, os.MkdirAll(filepath.Dir(corruptedVersionLocation), 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(corruptedVersionLocation),
		[]byte("not an executable"),
		0755))

	// Create two valid updates in the library
	olderValidVersion := "0.13.5"
	middleValidVersion := "1.0.5-11-abcdabcd"
	newerValidVersion := "1.0.7"
	for _, v := range []string{olderValidVersion, middleValidVersion, newerValidVersion} {
		versionDir := filepath.Join(testBaseDir, "launcher", v)
		executablePath := executableLocation(versionDir, binaryLauncher)
		require.NoError(t, os.MkdirAll(filepath.Dir(executablePath), 0755))
		copyBinary(t, executablePath)
		require.NoError(t, os.Chmod(executablePath, 0755))
		_, err := os.Stat(executablePath)
		require.NoError(t, err, "did not create binary for test")
		require.NoError(t, autoupdate.CheckExecutable(context.TODO(), executablePath, "--version"), "binary created for test is corrupt")
	}

	// Set up test library
	testReadOnlyLibrary, err := newReadOnlyLibrary(testBaseDir, log.NewNopLogger())
	require.NoError(t, err, "unexpected error creating new read-only library")

	// Get sorted versions
	validVersions, invalidVersions, err := testReadOnlyLibrary.sortedVersionsInLibrary(binaryLauncher)
	require.NoError(t, err, "expected no error on sorting versions in library")

	// Confirm invalid versions are the ones we expect
	require.Equal(t, 2, len(invalidVersions))
	require.Contains(t, invalidVersions, invalidVersion)
	require.Contains(t, invalidVersions, corruptedVersion)

	// Confirm valid versions are the ones we expect and that they're sorted in ascending order
	require.Equal(t, 3, len(validVersions))
	require.Equal(t, olderValidVersion, validVersions[0], "not sorted")
	require.Equal(t, middleValidVersion, validVersions[1], "not sorted")
	require.Equal(t, newerValidVersion, validVersions[2], "not sorted")
}

func Test_installedVersion_cached(t *testing.T) {
	t.Parallel()

	for _, binary := range binaries {
		binary := binary
		t.Run(string(binary), func(t *testing.T) {
			// Create update directories
			testBaseDir := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(testBaseDir, "launcher"), 0755))
			require.NoError(t, os.MkdirAll(filepath.Join(testBaseDir, "osqueryd"), 0755))

			// Set up test library
			testReadOnlyLibrary, err := newReadOnlyLibrary(testBaseDir, log.NewNopLogger())
			require.NoError(t, err, "unexpected error creating new read-only library")

			// Create cached version file
			expectedVersion := "5.5.5"
			require.NoError(t, os.WriteFile(
				filepath.Join(testReadOnlyLibrary.baseDir, fmt.Sprintf("%s-installed-version", binary)),
				[]byte(expectedVersion),
				0755))

			// Create fake executable in current working directory
			executablePath, err := os.Executable()
			require.NoError(t, err)
			testExecutablePath := executableLocation(filepath.Dir(executablePath), binary)
			require.NoError(t, os.MkdirAll(filepath.Dir(testExecutablePath), 0755))
			require.NoError(t, os.WriteFile(testExecutablePath, []byte("test"), 0755))
			t.Cleanup(func() {
				os.Remove(testExecutablePath)
			})

			actualVersion, actualPath, err := testReadOnlyLibrary.installedVersion(binary)
			require.NoError(t, err, "could not get installed version")
			require.Equal(t, expectedVersion, actualVersion.Original(), "version mismatch")
			require.Equal(t, testExecutablePath, actualPath)
		})
	}
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

func Test_parseLauncherVersion(t *testing.T) {
	t.Parallel()

	launcherVersionOutputDev := `launcher - version 1.0.7-45-g2abfc88-dirty
	branch: 	becca/tuf-find-new-v2
	revision: 	2abfc8883b96460603b49bc6f5cc44d5756890cf
	build date: 	2023-05-04
	build user: 	System Administrator (root)
	go version: 	go1.19.5`
	devVersion, err := parseLauncherVersion([]byte(launcherVersionOutputDev))
	require.NoError(t, err, "should be able to parse launcher dev version without error")
	require.NotNil(t, devVersion, "should have been able to parse launcher dev version as semver")
	require.Equal(t, "1.0.7-45-g2abfc88-dirty", devVersion.Original(), "dev semver should match")

	launcherVersionOutputNightly := `{"caller":"main.go:30","msg":"Launcher starting up","revision":"3e305bdb54c301759b62e9038faaa2cfea8abad1","severity":"info","ts":"2023-05-04T17:00:34.564523Z","version":"0.13.5-11-g3e305bd"}
	launcher - version 0.13.5-11-g3e305bd
	  branch: 	main
	  revision: 	3e305bdb54c301759b62e9038faaa2cfea8abad1
	  build date: 	2023-02-14
	  build user: 	runner (runner)
	  go version: 	go1.19.4`
	nightlyVersion, err := parseLauncherVersion([]byte(launcherVersionOutputNightly))
	require.NoError(t, err, "should be able to parse launcher nightly version without error")
	require.NotNil(t, nightlyVersion, "should have been able to parse launcher nightly version as semver")
	require.Equal(t, "0.13.5-11-g3e305bd", nightlyVersion.Original(), "nightly semver should match")

	launcherVersionOutputStable := `launcher - version 1.0.3
	  branch: 	main
	  revision: 	3e305bdb54c301759b62e9038faaa2cfea8abad1
	  build date: 	2023-02-14
	  build user: 	runner (runner)
	  go version: 	go1.19.4`
	stableVersion, err := parseLauncherVersion([]byte(launcherVersionOutputStable))
	require.NoError(t, err, "should be able to parse launcher stable version without error")
	require.NotNil(t, stableVersion, "should have been able to parse launcher stable version as semver")
	require.Equal(t, "1.0.3", stableVersion.Original(), "stable semver should match")
}

func Test_parseOsquerydVersion(t *testing.T) {
	t.Parallel()

	osquerydVersionOutput := `osqueryd version 5.8.1`

	v, err := parseOsquerydVersion([]byte(osquerydVersionOutput))
	require.NoError(t, err, "should be able to parse osqueryd version without error")
	require.NotNil(t, v, "should have been able to parse osqueryd version as semver")
	require.Equal(t, "5.8.1", v.Original(), "osqueryd semver should match")
}
