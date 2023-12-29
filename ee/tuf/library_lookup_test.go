package tuf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kit/kit/log"
	tufci "github.com/kolide/launcher/ee/tuf/ci"
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
			updateDir := DefaultLibraryDirectory(rootDir)

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
			latest, err := CheckOutLatest(context.TODO(), binary, rootDir, "", "nightly", log.NewNopLogger())
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
			updateDir := DefaultLibraryDirectory(rootDir)
			target := fmt.Sprintf("%s-1.1.1.tar.gz", binary)
			executablePath, executableVersion := pathToTargetVersionExecutable(binary, target, updateDir)
			require.NoError(t, os.MkdirAll(filepath.Dir(executablePath), 0755))
			tufci.CopyBinary(t, executablePath)
			require.NoError(t, os.Chmod(executablePath, 0755))
			_, err := os.Stat(executablePath)
			require.NoError(t, err, "did not make test binary")

			// Check it
			latest, err := CheckOutLatest(context.TODO(), binary, rootDir, "", "nightly", log.NewNopLogger())
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

				_, err := CheckOutLatest(context.TODO(), binary, rootDir, "", channel, log.NewNopLogger())
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

			latest, err := mostRecentVersion(context.TODO(), binary, testBaseDir)
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

			latest, err := mostRecentVersion(context.TODO(), binary, testBaseDir)
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

			_, err := mostRecentVersion(context.TODO(), binary, testBaseDir)
			require.Error(t, err, "should have returned error when there are no available updates")
		})
	}
}

func TestChannelUsesNewAutoupdater(t *testing.T) {
	t.Parallel()

	channelsForTest := []struct {
		channelName        string
		usesNewAutoupdater bool
	}{
		{
			channelName:        "nightly",
			usesNewAutoupdater: true,
		},
		{
			channelName:        "alpha",
			usesNewAutoupdater: true,
		},
		{
			channelName:        "stable",
			usesNewAutoupdater: false,
		},
		{
			channelName:        "beta",
			usesNewAutoupdater: true,
		},
		{
			channelName:        "",
			usesNewAutoupdater: false,
		},
	}

	for _, channel := range channelsForTest {
		require.Equal(t, channel.usesNewAutoupdater, ChannelUsesNewAutoupdater(channel.channelName))
	}
}

func Test_getAutoupdateConfig_ConfigFlagSet(t *testing.T) {
	t.Parallel()

	tempConfDir := t.TempDir()
	configFilepath := filepath.Join(tempConfDir, "launcher.flags")

	testRootDir := t.TempDir()
	testChannel := "nightly"

	fileContents := fmt.Sprintf(`
with_initial_runner
autoupdate
hostname localhost
root_directory %s
update_channel %s
transport jsonrpc
`,
		testRootDir,
		testChannel,
	)

	require.NoError(t, os.WriteFile(configFilepath, []byte(fileContents), 0755), "expected to set up test config file")

	cfg, err := getAutoupdateConfig([]string{"--config", configFilepath})
	require.NoError(t, err, "expected no error getting autoupdate config")

	require.NotNil(t, cfg, "expected valid autoupdate config")
	require.Equal(t, testRootDir, cfg.rootDirectory, "root directory is incorrect")
	require.Equal(t, "", cfg.updateDirectory, "update directory should not have been set")
	require.Equal(t, testChannel, cfg.channel, "channel is incorrect")
	require.Equal(t, "", cfg.localDevelopmentPath, "local development path should not have been set")
}

func Test_getAutoupdateConfig_ConfigFlagNotSet(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()
	testUpdateDir := t.TempDir()
	testChannel := "nightly"
	testLocaldevPath := filepath.Join("some", "path", "to", "a", "local", "build")

	cfg, err := getAutoupdateConfig([]string{
		"--root_directory", testRootDir,
		"--osquery_flag", "enable_watchdog_debug=true",
		"--update_directory", testUpdateDir,
		"--autoupdate",
		"--update_channel", testChannel,
		"--localdev_path", testLocaldevPath,
		"--transport", "jsonrpc",
	})
	require.NoError(t, err, "expected no error getting autoupdate config")

	require.NotNil(t, cfg, "expected valid autoupdate config")
	require.Equal(t, testRootDir, cfg.rootDirectory, "root directory is incorrect")
	require.Equal(t, testUpdateDir, cfg.updateDirectory, "update directory is incorrect")
	require.Equal(t, testChannel, cfg.channel, "channel is incorrect")
	require.Equal(t, testLocaldevPath, cfg.localDevelopmentPath, "local development path is incorrect")
}
