package tuf

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_newUpdateLibraryManager(t *testing.T) {
	t.Parallel()
	t.Skip("TODO")
}

func Test_addToLibrary(t *testing.T) {
	t.Parallel()
	t.Skip("TODO")
}

func Test_addToLibrary_alreadyAdded(t *testing.T) {
	t.Parallel()
	t.Skip("TODO")
}

func Test_addToLibrary_verifyStagedUpdate_handlesInvalidFiles(t *testing.T) {
	t.Parallel()
	t.Skip("TODO")
}

func Test_verifyExecutable(t *testing.T) {
	t.Parallel()
	t.Skip("TODO")
}

func Test_tidyLibrary(t *testing.T) {
	t.Parallel()
	t.Skip("TODO")
}

func Test_versionFromTarget(t *testing.T) {
	t.Parallel()

	testVersions := []struct {
		target          string
		binary          string
		operatingSystem string
		version         string
	}{
		{
			target:          "launcher/darwin/launcher-0.10.1.tar.gz",
			binary:          "launcher",
			operatingSystem: "darwin",
			version:         "0.10.1",
		},
		{
			target:          "launcher/windows/launcher-1.13.5.tar.gz",
			binary:          "launcher",
			operatingSystem: "windows",
			version:         "1.13.5",
		},
		{
			target:          "launcher/linux/launcher-0.13.5-40-gefdc582.tar.gz",
			binary:          "launcher",
			operatingSystem: "linux",
			version:         "0.13.5-40-gefdc582",
		},
		{
			target:          "osqueryd/darwin/osqueryd-5.8.1.tar.gz",
			binary:          "osqueryd",
			operatingSystem: "darwin",
			version:         "5.8.1",
		},
		{
			target:          "osqueryd/windows/osqueryd-0.8.1.tar.gz",
			binary:          "osqueryd",
			operatingSystem: "windows",
			version:         "0.8.1",
		},
		{
			target:          "osqueryd/linux/osqueryd-5.8.2.tar.gz",
			binary:          "osqueryd",
			operatingSystem: "linux",
			version:         "5.8.2",
		},
	}

	for _, testVersion := range testVersions {
		libManager := &updateLibraryManager{
			operatingSystem: testVersion.operatingSystem,
		}
		require.Equal(t, testVersion.version, libManager.versionFromTarget(testVersion.binary, filepath.Base(testVersion.target)))
	}
}
