package tuf

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/go-kit/kit/log"
	tufci "github.com/kolide/launcher/pkg/autoupdate/tuf/ci"
	"github.com/stretchr/testify/require"
)

func TestCheckOutLatest_withTufRepository(t *testing.T) {
	t.Parallel()

	for _, binary := range binaries {
		binary := binary
		t.Run(string(binary), func(t *testing.T) {
			t.Parallel()

			// Set up an update library
			rootDir := t.TempDir()
			updateDir := defaultLibraryDirectory(rootDir)

			// Set up a local TUF repo
			tufDir := LocalTufDirectory(rootDir)
			require.NoError(t, os.MkdirAll(tufDir, 488))
			testReleaseVersion := "1.0.30"
			expectedTargetName := fmt.Sprintf("%s-%s.tar.gz", binary, testReleaseVersion)
			tufci.SeedLocalTufRepo(t, testReleaseVersion, rootDir)

			// Create a corresponding downloaded target
			executablePath, executableVersion := pathToTargetVersionExecutable(binary, expectedTargetName, updateDir)
			require.NoError(t, os.MkdirAll(filepath.Dir(executablePath), 0755))
			tufci.CopyBinary(t, executablePath)
			require.NoError(t, os.Chmod(executablePath, 0755))

			// Make a more recent version that we should ignore since it isn't the release version
			tooRecentTarget := fmt.Sprintf("%s-2.1.1.tar.gz", binary)
			tooRecentPath, _ := pathToTargetVersionExecutable(binary, tooRecentTarget, updateDir)
			require.NoError(t, os.MkdirAll(filepath.Dir(tooRecentPath), 0755))
			tufci.CopyBinary(t, tooRecentPath)
			require.NoError(t, os.Chmod(tooRecentPath, 0755))

			// Check it
			latest, err := CheckOutLatest(binary, rootDir, "", "nightly", log.NewNopLogger())
			require.NoError(t, err, "unexpected error on checking out latest")
			require.Equal(t, executablePath, latest.Path)
			require.Equal(t, executableVersion, latest.Version)
		})
	}
}

func TestCheckOutLatest_withoutTufRepository(t *testing.T) {
	t.Parallel()

	for _, binary := range binaries {
		binary := binary
		t.Run(string(binary), func(t *testing.T) {
			t.Parallel()

			// Set up an update library, but no TUF repo
			rootDir := t.TempDir()
			updateDir := defaultLibraryDirectory(rootDir)
			target := fmt.Sprintf("%s-1.1.1.tar.gz", binary)
			executablePath, executableVersion := pathToTargetVersionExecutable(binary, target, updateDir)
			require.NoError(t, os.MkdirAll(filepath.Dir(executablePath), 0755))
			tufci.CopyBinary(t, executablePath)
			require.NoError(t, os.Chmod(executablePath, 0755))
			_, err := os.Stat(executablePath)
			require.NoError(t, err, "did not make test binary")

			// Check it
			latest, err := CheckOutLatest(binary, rootDir, "", "nightly", log.NewNopLogger())
			require.NoError(t, err, "unexpected error on checking out latest")
			require.Equal(t, executablePath, latest.Path)
			require.Equal(t, executableVersion, latest.Version)
		})
	}
}

func TestCheckOutLatest_NotAvailableOnNonNightlyChannels(t *testing.T) {
	t.Parallel()

	for _, binary := range binaries {
		binary := binary
		for _, channel := range []string{"beta", "stable"} {
			channel := channel
			t.Run(fmt.Sprintf("%s-%s", binary, channel), func(t *testing.T) {
				t.Parallel()

				rootDir := t.TempDir()

				_, err := CheckOutLatest(binary, rootDir, "", channel, log.NewNopLogger())
				require.Error(t, err, "expected error when using new TUF lookup on channel that should be using legacy")
			})
		}
	}
}

func Test_mostRecentVersion(t *testing.T) {
	t.Parallel()

	for _, binary := range binaries {
		binary := binary
		t.Run(string(binary), func(t *testing.T) {
			t.Parallel()

			// Create update directories
			testBaseDir := t.TempDir()

			// Now, create a version in the update library
			firstVersionTarget := fmt.Sprintf("%s-2.2.3.tar.gz", binary)
			firstVersionPath, _ := pathToTargetVersionExecutable(binary, firstVersionTarget, testBaseDir)
			require.NoError(t, os.MkdirAll(filepath.Dir(firstVersionPath), 0755))
			tufci.CopyBinary(t, firstVersionPath)
			require.NoError(t, os.Chmod(firstVersionPath, 0755))

			// Create an even newer version in the update library
			secondVersionTarget := fmt.Sprintf("%s-2.5.3.tar.gz", binary)
			secondVersionPath, secondVersion := pathToTargetVersionExecutable(binary, secondVersionTarget, testBaseDir)
			require.NoError(t, os.MkdirAll(filepath.Dir(secondVersionPath), 0755))
			tufci.CopyBinary(t, secondVersionPath)
			require.NoError(t, os.Chmod(secondVersionPath, 0755))

			latest, err := mostRecentVersion(binary, testBaseDir)
			require.NoError(t, err, "did not expect error getting most recent version")
			require.Equal(t, secondVersionPath, latest.Path)
			require.Equal(t, secondVersion, latest.Version)
		})
	}
}

func Test_mostRecentVersion_DoesNotReturnInvalidExecutables(t *testing.T) {
	t.Parallel()

	for _, binary := range binaries {
		binary := binary
		t.Run(string(binary), func(t *testing.T) {
			t.Parallel()

			// Create update directories
			testBaseDir := t.TempDir()

			// Now, create a version in the update library
			firstVersionTarget := fmt.Sprintf("%s-2.2.3.tar.gz", binary)
			firstVersionPath, firstVersion := pathToTargetVersionExecutable(binary, firstVersionTarget, testBaseDir)
			require.NoError(t, os.MkdirAll(filepath.Dir(firstVersionPath), 0755))
			tufci.CopyBinary(t, firstVersionPath)
			require.NoError(t, os.Chmod(firstVersionPath, 0755))

			// Create an even newer, but also corrupt, version in the update library
			secondVersionTarget := fmt.Sprintf("%s-2.1.12.tar.gz", binary)
			secondVersionPath, _ := pathToTargetVersionExecutable(binary, secondVersionTarget, testBaseDir)
			require.NoError(t, os.MkdirAll(filepath.Dir(secondVersionPath), 0755))
			os.WriteFile(secondVersionPath, []byte{}, 0755)

			latest, err := mostRecentVersion(binary, testBaseDir)
			require.NoError(t, err, "did not expect error getting most recent version")
			require.Equal(t, firstVersionPath, latest.Path)
			require.Equal(t, firstVersion, latest.Version)
		})
	}
}

func Test_mostRecentVersion_ReturnsErrorOnNoUpdatesDownloaded(t *testing.T) {
	t.Parallel()

	for _, binary := range binaries {
		binary := binary
		t.Run(string(binary), func(t *testing.T) {
			t.Parallel()

			// Create update directories
			testBaseDir := t.TempDir()

			_, err := mostRecentVersion(binary, testBaseDir)
			require.Error(t, err, "should have returned error when there are no available updates")
		})
	}
}

func Test_getConfigFilePath(t *testing.T) {
	t.Parallel()

	var fallbackConfigFile string
	switch runtime.GOOS {
	case "darwin", "linux":
		fallbackConfigFile = "/etc/kolide-k2/launcher.flags"
	case "windows":
		fallbackConfigFile = `C:\Program Files\Kolide\Launcher-kolide-k2\conf\launcher.flags`
	}

	testCases := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name: "single hyphen",
			args: []string{
				"-some", "arg",
				"-config", "/single/hyphen/path/to/launcher.flags",
				"-another", "arg",
			},
			expected: "/single/hyphen/path/to/launcher.flags",
		},
		{
			name: "double hyphen",
			args: []string{
				"--config", "/double/hyphen/path/to/launcher.flags",
				"--some", "arg",
			},
			expected: "/double/hyphen/path/to/launcher.flags",
		},
		{
			name: "double hyphen and equals",
			args: []string{
				"--arg1=value",
				"--config=/different/path/to/launcher.flags",
			},
			expected: "/different/path/to/launcher.flags",
		},
		{
			name:     "no config file present",
			args:     []string{},
			expected: fallbackConfigFile,
		},
	}

	for _, tt := range testCases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, getConfigFilePath(tt.args), tt.expected)
		})
	}
}
