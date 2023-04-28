package tuf

import (
	"fmt"
	"os"
	"path/filepath"
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

	testBaseDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(testBaseDir, "launcher"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(testBaseDir, "osqueryd"), 0755))

	testReadOnlyLibrary, err := newReadOnlyLibrary(testBaseDir, log.NewNopLogger())
	require.NoError(t, err, "unexpected error creating new update library manager")

	// Query for the current osquery version
	runningOsqueryVersion := "5.5.7"
	require.True(t, testReadOnlyLibrary.Available(binaryOsqueryd, fmt.Sprintf("osqueryd-%s.tar.gz", runningOsqueryVersion)))

	// Query for a different osqueryd version
	require.False(t, testReadOnlyLibrary.Available(binaryOsqueryd, "osqueryd-5.6.7.tar.gz"))
}

func Test_currentRunningVersion_launcher_errorWhenVersionIsNotSet(t *testing.T) {
	t.Parallel()

	testReadOnlyLibrary := &readOnlyLibrary{
		logger: log.NewNopLogger(),
	}

	// In test, version.Version() returns `unknown` for everything, which is not something
	// that the semver library can parse. So we only expect an error here.
	launcherVersion, err := testReadOnlyLibrary.currentRunningVersion("launcher")
	require.Error(t, err, "expected an error fetching current running version of launcher")
	require.Equal(t, "", launcherVersion)
}

func Test_currentRunningVersion_osqueryd(t *testing.T) {
	t.Parallel()

	testReadOnlyLibrary := &readOnlyLibrary{
		logger: log.NewNopLogger(),
	}

	expectedOsqueryVersion, err := semver.NewVersion("5.10.12")
	require.NoError(t, err)

	osqueryVersion, err := testReadOnlyLibrary.currentRunningVersion("osqueryd")
	require.NoError(t, err, "expected no error fetching current running version of osqueryd")
	require.Equal(t, expectedOsqueryVersion.Original(), osqueryVersion)
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
		testReadOnlyLibrary := &readOnlyLibrary{}
		require.Equal(t, testVersion.version, testReadOnlyLibrary.versionFromTarget(testVersion.binary, filepath.Base(testVersion.target)))
	}
}
