package tuf

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/kolide/launcher/ee/agent/flags/keys"
	agentsqlite "github.com/kolide/launcher/ee/agent/storage/sqlite"
	tufci "github.com/kolide/launcher/ee/tuf/ci"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func Test_getUpdateSettingsFromStartupSettings(t *testing.T) {
	t.Parallel()

	expectedChannel := "beta"
	expectedPinnedVersion := "1.5.5"

	// Set up an override for the channel in the startupsettings db
	rootDir := t.TempDir()
	store, err := agentsqlite.OpenRW(t.Context(), rootDir, agentsqlite.StartupSettingsStore)
	require.NoError(t, err, "setting up db connection")
	require.NoError(t, store.Set([]byte(keys.UpdateChannel.String()), []byte(expectedChannel)), "setting key")
	require.NoError(t, store.Set([]byte(keys.PinnedLauncherVersion.String()), []byte(expectedPinnedVersion)), "setting key")
	require.NoError(t, store.Set([]byte(keys.PinnedOsquerydVersion.String()), []byte("5.5.5")), "setting key")
	require.NoError(t, store.Close(), "closing test db")

	actualVersion, actualChannel, err := getUpdateSettingsFromStartupSettings(t.Context(), multislogger.NewNopLogger(), "launcher", rootDir)
	require.NoError(t, err, "did not expect error getting update settings from startup settings")
	require.Equal(t, expectedPinnedVersion, actualVersion, "did not get expected version")
	require.Equal(t, expectedChannel, actualChannel, "did not get expected channel")
}

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
			latest, err := CheckOutLatest(t.Context(), binary, rootDir, "", "", "nightly", multislogger.NewNopLogger())
			require.NoError(t, err, "unexpected error on checking out latest")
			require.Equal(t, executablePath, latest.Path)
			require.Equal(t, executableVersion, latest.Version)
		})
	}
}

func TestCheckOutLatest_withTufRepository_withPinnedVersion(t *testing.T) {
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
			pinnedVersion := tufci.NonReleaseVersion
			expectedTargetName := fmt.Sprintf("%s-%s.tar.gz", binary, pinnedVersion)
			testReleaseVersion := "2.3.3"
			tufci.SeedLocalTufRepo(t, testReleaseVersion, rootDir)

			// Create a corresponding downloaded target for the pinned version
			executablePath, executableVersion := pathToTargetVersionExecutable(binary, expectedTargetName, updateDir)
			require.NoError(t, os.MkdirAll(filepath.Dir(executablePath), 0755))
			tufci.CopyBinary(t, executablePath)
			require.NoError(t, os.Chmod(executablePath, 0755))

			// Make a more recent version that we should ignore since it isn't the pinned version
			releaseTarget := fmt.Sprintf("%s-%s.tar.gz", binary, testReleaseVersion)
			releaseTargetPath, _ := pathToTargetVersionExecutable(binary, releaseTarget, updateDir)
			require.NoError(t, os.MkdirAll(filepath.Dir(releaseTargetPath), 0755))
			tufci.CopyBinary(t, releaseTargetPath)
			require.NoError(t, os.Chmod(releaseTargetPath, 0755))

			// Check it
			latest, err := CheckOutLatest(t.Context(), binary, rootDir, "", pinnedVersion, "nightly", multislogger.NewNopLogger())
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
			latest, err := CheckOutLatest(t.Context(), binary, rootDir, "", "", "nightly", multislogger.NewNopLogger())
			require.NoError(t, err, "unexpected error on checking out latest")
			require.Equal(t, executablePath, latest.Path)
			require.Equal(t, executableVersion, latest.Version)
		})
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

			latest, err := mostRecentVersion(t.Context(), multislogger.NewNopLogger(), binary, testBaseDir, "nightly")
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

			latest, err := mostRecentVersion(t.Context(), multislogger.NewNopLogger(), binary, testBaseDir, "nightly")
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

			_, err := mostRecentVersion(t.Context(), multislogger.NewNopLogger(), binary, testBaseDir, "nightly")
			require.Error(t, err, "should have returned error when there are no available updates")
		})
	}
}

func Test_mostRecentVersion_requiresLauncher_v1_4_1(t *testing.T) {
	t.Parallel()

	testBaseDir := t.TempDir()

	// Create a version in the update library that is too old
	firstVersionTarget := "launcher-1.2.3.tar.gz"
	firstVersionPath, _ := pathToTargetVersionExecutable(binaryLauncher, firstVersionTarget, testBaseDir)
	require.NoError(t, os.MkdirAll(filepath.Dir(firstVersionPath), 0755))
	tufci.CopyBinary(t, firstVersionPath)
	require.NoError(t, os.Chmod(firstVersionPath, 0755))

	_, err := mostRecentVersion(t.Context(), multislogger.NewNopLogger(), binaryLauncher, testBaseDir, "stable")
	require.Error(t, err, "should not select launcher version under v1.4.1")
}

func Test_mostRecentVersion_acceptsLauncher_v1_4_1(t *testing.T) {
	t.Parallel()

	testBaseDir := t.TempDir()

	// Create a version in the update library that is too old
	firstVersionTarget := "launcher-1.4.1.tar.gz"
	firstVersionPath, _ := pathToTargetVersionExecutable(binaryLauncher, firstVersionTarget, testBaseDir)
	require.NoError(t, os.MkdirAll(filepath.Dir(firstVersionPath), 0755))
	tufci.CopyBinary(t, firstVersionPath)
	require.NoError(t, os.Chmod(firstVersionPath, 0755))

	latest, err := mostRecentVersion(t.Context(), multislogger.NewNopLogger(), binaryLauncher, testBaseDir, "stable")
	require.NoError(t, err, "should be able to select launcher version equal to v1.4.1")
	require.Equal(t, firstVersionPath, latest.Path)
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

	cfg1, err := getAutoupdateConfig([]string{"--config", configFilepath})
	require.NoError(t, err, "expected no error getting autoupdate config")

	require.NotNil(t, cfg1, "expected valid autoupdate config")
	require.Equal(t, testRootDir, cfg1.rootDirectory, "root directory is incorrect")
	require.Equal(t, "", cfg1.updateDirectory, "update directory should not have been set")
	require.Equal(t, testChannel, cfg1.channel, "channel is incorrect")
	require.Equal(t, "", cfg1.localDevelopmentPath, "local development path should not have been set")

	// Same thing, just one - instead of 2
	cfg2, err := getAutoupdateConfig([]string{"-config", configFilepath})
	require.NoError(t, err, "expected no error getting autoupdate config")

	require.NotNil(t, cfg2, "expected valid autoupdate config")
	require.Equal(t, testRootDir, cfg2.rootDirectory, "root directory is incorrect")
	require.Equal(t, "", cfg2.updateDirectory, "update directory should not have been set")
	require.Equal(t, testChannel, cfg2.channel, "channel is incorrect")
	require.Equal(t, "", cfg2.localDevelopmentPath, "local development path should not have been set")
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

func Test_getAutoupdateConfig_ConfigFlagNotSet_SingleHyphen(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()
	testUpdateDir := t.TempDir()
	testChannel := "nightly"
	testLocaldevPath := filepath.Join("some", "path", "to", "a", "local", "build")

	cfg, err := getAutoupdateConfig([]string{
		"-root_directory", testRootDir,
		"-osquery_flag", "enable_watchdog_debug=true",
		"-update_directory", testUpdateDir,
		"-autoupdate",
		"-update_channel", testChannel,
		"-localdev_path", testLocaldevPath,
		"-transport", "jsonrpc",
	})
	require.NoError(t, err, "expected no error getting autoupdate config")

	require.NotNil(t, cfg, "expected valid autoupdate config")
	require.Equal(t, testRootDir, cfg.rootDirectory, "root directory is incorrect")
	require.Equal(t, testUpdateDir, cfg.updateDirectory, "update directory is incorrect")
	require.Equal(t, testChannel, cfg.channel, "channel is incorrect")
	require.Equal(t, testLocaldevPath, cfg.localDevelopmentPath, "local development path is incorrect")
}
