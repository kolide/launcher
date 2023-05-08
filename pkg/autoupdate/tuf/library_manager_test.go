package tuf

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/Masterminds/semver"
	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/autoupdate"
	"github.com/stretchr/testify/require"
	"github.com/theupdateframework/go-tuf/data"
)

func Test_newUpdateLibraryManager(t *testing.T) {
	t.Parallel()

	testBaseDir := filepath.Join(t.TempDir(), "updates")
	testLibraryManager, err := newUpdateLibraryManager("", nil, testBaseDir, log.NewNopLogger())
	require.NoError(t, err, "unexpected error creating new update library manager")

	baseDir, err := os.Stat(testBaseDir)
	require.NoError(t, err, "could not stat base dir")
	require.True(t, baseDir.IsDir(), "base dir is not a directory")

	stagedDownloadDir, err := os.Stat(testLibraryManager.stagingDir)
	require.NoError(t, err, "could not stat staged osqueryd download dir")
	require.True(t, stagedDownloadDir.IsDir(), "staged osqueryd download dir is not a directory")

	osquerydDownloadDir, err := os.Stat(filepath.Join(testBaseDir, "osqueryd"))
	require.NoError(t, err, "could not stat osqueryd download dir")
	require.True(t, osquerydDownloadDir.IsDir(), "osqueryd download dir is not a directory")

	launcherDownloadDir, err := os.Stat(filepath.Join(testBaseDir, "launcher"))
	require.NoError(t, err, "could not stat launcher download dir")
	require.True(t, launcherDownloadDir.IsDir(), "launcher download dir is not a directory")
}

func TestPathToTargetVersionExecutable(t *testing.T) {
	t.Parallel()

	testBaseDir := filepath.Join(t.TempDir(), "updates")
	testLibrary, err := newUpdateLibraryManager("", nil, testBaseDir, log.NewNopLogger())
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

func TestMostRecentVersion(t *testing.T) {
	t.Parallel()

	// Create update directories
	testBaseDir := t.TempDir()

	// Set up test library
	testLibrary, err := newUpdateLibraryManager("", nil, testBaseDir, log.NewNopLogger())
	require.NoError(t, err, "unexpected error creating new library")

	for _, binary := range binaries {
		// First, create an old install version
		installVersion, err := semver.NewVersion("1.0.4")
		require.NoError(t, err, "unexpected error making semver")
		testLibrary.cacheInstalledVersion(binary, installVersion)

		// Now, create a version in the update library
		firstVersionTarget := fmt.Sprintf("%s-2.2.3.tar.gz", binary)
		firstVersionPath := testLibrary.PathToTargetVersionExecutable(binary, firstVersionTarget)
		require.NoError(t, os.MkdirAll(filepath.Dir(firstVersionPath), 0755))
		copyBinary(t, firstVersionPath)
		require.NoError(t, os.Chmod(firstVersionPath, 0755))

		// Create an even newer version in the update library
		secondVersionTarget := fmt.Sprintf("%s-2.5.3.tar.gz", binary)
		secondVersionPath := testLibrary.PathToTargetVersionExecutable(binary, secondVersionTarget)
		require.NoError(t, os.MkdirAll(filepath.Dir(secondVersionPath), 0755))
		copyBinary(t, secondVersionPath)
		require.NoError(t, os.Chmod(secondVersionPath, 0755))

		pathToVersion, err := testLibrary.MostRecentVersion(binary)
		require.NoError(t, err, "did not expect error getting most recent version")
		require.Equal(t, secondVersionPath, pathToVersion)
	}
}

func TestMostRecentVersion_DoesNotReturnInvalidExecutables(t *testing.T) {
	t.Parallel()

	// Create update directories
	testBaseDir := t.TempDir()

	// Set up test library
	testLibrary, err := newUpdateLibraryManager("", nil, testBaseDir, log.NewNopLogger())
	require.NoError(t, err, "unexpected error creating new library")

	for _, binary := range binaries {
		// First, create an old install version
		installVersion, err := semver.NewVersion("1.0.4")
		require.NoError(t, err, "unexpected error making semver")
		testLibrary.cacheInstalledVersion(binary, installVersion)

		// Now, create a version in the update library
		firstVersionTarget := fmt.Sprintf("%s-2.2.3.tar.gz", binary)
		firstVersionPath := testLibrary.PathToTargetVersionExecutable(binary, firstVersionTarget)
		require.NoError(t, os.MkdirAll(filepath.Dir(firstVersionPath), 0755))
		copyBinary(t, firstVersionPath)
		require.NoError(t, os.Chmod(firstVersionPath, 0755))

		// Create an even newer, but also corrupt, version in the update library
		secondVersionTarget := fmt.Sprintf("%s-2.1.12.tar.gz", binary)
		secondVersionPath := testLibrary.PathToTargetVersionExecutable(binary, secondVersionTarget)
		require.NoError(t, os.MkdirAll(filepath.Dir(secondVersionPath), 0755))
		os.WriteFile(secondVersionPath, []byte{}, 0755)

		pathToVersion, err := testLibrary.MostRecentVersion(binary)
		require.NoError(t, err, "did not expect error getting most recent version")
		require.Equal(t, firstVersionPath, pathToVersion)
	}
}

func TestMostRecentVersion_InstalledVersionIsMostRecent(t *testing.T) {
	t.Parallel()

	// Create update directories
	testBaseDir := t.TempDir()

	// Set up test library
	testLibrary, err := newUpdateLibraryManager("", nil, testBaseDir, log.NewNopLogger())
	require.NoError(t, err, "unexpected error creating new library")

	for _, binary := range binaries {
		// Create a version in the update library
		firstVersionTarget := fmt.Sprintf("%s-3.1.3.tar.gz", binary)
		firstVersionPath := testLibrary.PathToTargetVersionExecutable(binary, firstVersionTarget)
		require.NoError(t, os.MkdirAll(filepath.Dir(firstVersionPath), 0755))
		copyBinary(t, firstVersionPath)
		require.NoError(t, os.Chmod(firstVersionPath, 0755))

		// Create an even newer version in the update library
		secondVersionTarget := fmt.Sprintf("%s-3.6.3.tar.gz", binary)
		secondVersionPath := testLibrary.PathToTargetVersionExecutable(binary, secondVersionTarget)
		require.NoError(t, os.MkdirAll(filepath.Dir(secondVersionPath), 0755))
		copyBinary(t, secondVersionPath)
		require.NoError(t, os.Chmod(secondVersionPath, 0755))

		// Create an install version that is even newer
		installVersion, err := semver.NewVersion("3.10.4")
		require.NoError(t, err, "unexpected error making semver")
		testLibrary.cacheInstalledVersion(binary, installVersion)

		// Create fake executable in current working directory to stand in for installed path
		executablePath, err := os.Executable()
		require.NoError(t, err)
		testExecutablePath := filepath.Join(filepath.Dir(executablePath), string(binary))
		require.NoError(t, os.WriteFile(testExecutablePath, []byte("test"), 0755))
		t.Cleanup(func() {
			os.Remove(testExecutablePath)
		})

		pathToVersion, err := testLibrary.MostRecentVersion(binary)
		require.NoError(t, err, "did not expect error getting most recent version")

		// We don't do an exact comparison with `testExecutablePath` because running tests locally
		// will pick up real install locations first. Instead, confirm the path isn't empty and that
		// it's not either of the update paths.
		require.NotEqual(t, "", pathToVersion)
		require.NotEqual(t, firstVersionPath, pathToVersion)
		require.NotEqual(t, secondVersionPath, pathToVersion)
	}
}

func TestAvailable(t *testing.T) {
	t.Parallel()

	// Create update directories
	testBaseDir := t.TempDir()

	// Set up test library
	testLibrary, err := newUpdateLibraryManager("", nil, testBaseDir, log.NewNopLogger())
	require.NoError(t, err, "unexpected error creating new read-only library")

	// Set up valid "osquery" executable
	runningOsqueryVersion := "5.5.7"
	runningTarget := fmt.Sprintf("osqueryd-%s.tar.gz", runningOsqueryVersion)
	executablePath := testLibrary.PathToTargetVersionExecutable(binaryOsqueryd, runningTarget)
	copyBinary(t, executablePath)
	require.NoError(t, os.Chmod(executablePath, 0755))

	// Query for the current osquery version
	require.True(t, testLibrary.Available(binaryOsqueryd, runningTarget))

	// Query for a different osqueryd version
	require.False(t, testLibrary.Available(binaryOsqueryd, "osqueryd-5.6.7.tar.gz"))
}

func TestAddToLibrary(t *testing.T) {
	t.Parallel()

	// Set up TUF dependencies -- we do this here to avoid re-initializing the local tuf server for each
	// binary. It's unnecessary work since the mirror serves the same data both times.
	testBaseDir := t.TempDir()
	testReleaseVersion := "1.2.4"
	tufServerUrl, rootJson := initLocalTufServer(t, testReleaseVersion)
	metadataClient, err := initMetadataClient(testBaseDir, tufServerUrl, http.DefaultClient)
	require.NoError(t, err, "creating metadata client")
	// Re-initialize the metadata client with our test root JSON
	require.NoError(t, metadataClient.Init(rootJson), "could not initialize metadata client with test root JSON")
	_, err = metadataClient.Update()
	require.NoError(t, err, "could not update metadata client")

	// Get the target metadata
	launcherTargetMeta, err := metadataClient.Target(fmt.Sprintf("%s/%s/%s-%s.tar.gz", binaryLauncher, runtime.GOOS, binaryLauncher, testReleaseVersion))
	require.NoError(t, err, "could not get test metadata for launcher target")
	osquerydTargetMeta, err := metadataClient.Target(fmt.Sprintf("%s/%s/%s-%s.tar.gz", binaryOsqueryd, runtime.GOOS, binaryOsqueryd, testReleaseVersion))
	require.NoError(t, err, "could not get test metadata for launcher target")

	testCases := []struct {
		binary     autoupdatableBinary
		targetFile string
		targetMeta data.TargetFileMeta
	}{
		{
			binary:     binaryLauncher,
			targetFile: fmt.Sprintf("%s-%s.tar.gz", binaryLauncher, testReleaseVersion),
			targetMeta: launcherTargetMeta,
		},
		{
			binary:     binaryOsqueryd,
			targetFile: fmt.Sprintf("%s-%s.tar.gz", binaryOsqueryd, testReleaseVersion),
			targetMeta: osquerydTargetMeta,
		},
	}

	for _, tt := range testCases {
		tt := tt
		t.Run(string(tt.binary), func(t *testing.T) {
			t.Parallel()

			// Set up test library manager
			testLibraryManager, err := newUpdateLibraryManager(tufServerUrl, http.DefaultClient, testBaseDir, log.NewNopLogger())
			require.NoError(t, err, "unexpected error creating new update library manager")

			// Request download -- make a couple concurrent requests to confirm that the lock works.
			var wg sync.WaitGroup
			for i := 0; i < 5; i += 1 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					require.NoError(t, testLibraryManager.AddToLibrary(tt.binary, tt.targetFile, tt.targetMeta), "expected no error adding to library")
				}()
			}

			wg.Wait()

			// Confirm the update was downloaded
			dirInfo, err := os.Stat(filepath.Join(testLibraryManager.updatesDirectory(tt.binary), testReleaseVersion))
			require.NoError(t, err, "checking that update was downloaded")
			require.True(t, dirInfo.IsDir())
			executableInfo, err := os.Stat(executableLocation(filepath.Join(testLibraryManager.updatesDirectory(tt.binary), testReleaseVersion), tt.binary))
			require.NoError(t, err, "checking that downloaded update includes executable")
			require.False(t, executableInfo.IsDir())

			// Confirm the staging directory is empty
			matches, err := filepath.Glob(filepath.Join(testLibraryManager.stagingDir, "*"))
			require.NoError(t, err, "checking that staging dir was cleaned")
			require.Equal(t, 0, len(matches), "unexpected files found in staged updates directory: %+v", matches)
		})
	}
}

func TestAddToLibrary_alreadyInstalled(t *testing.T) {
	t.Parallel()

	for _, binary := range binaries {
		binary := binary
		t.Run(string(binary), func(t *testing.T) {
			t.Parallel()

			testBaseDir := t.TempDir()
			testMirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatalf("mirror should not have been called for download, but was: %s", r.URL.String())
			}))
			defer testMirror.Close()
			testLibraryManager, err := newUpdateLibraryManager(testMirror.URL, http.DefaultClient, testBaseDir, log.NewNopLogger())
			require.NoError(t, err, "initializing test library manager")

			// Make sure our update directories exist so we can verify they're empty later
			require.NoError(t, os.MkdirAll(testLibraryManager.updatesDirectory(binary), 0755))

			// Create cached installed version file
			testVersion := "0.12.1-abcdabcd"
			require.NoError(t, os.WriteFile(filepath.Join(testBaseDir, fmt.Sprintf("%s-installed-version", binary)), []byte(testVersion), 0755))

			// Create fake executable in current working directory
			executablePath, err := os.Executable()
			require.NoError(t, err)
			testExecutablePath := filepath.Join(filepath.Dir(executablePath), string(binary))
			require.NoError(t, os.WriteFile(testExecutablePath, []byte("test"), 0755))
			t.Cleanup(func() {
				os.Remove(testExecutablePath)
			})

			// Ask the library manager to perform the download
			require.NoError(t, testLibraryManager.AddToLibrary(binary, fmt.Sprintf("%s-%s.tar.gz", binary, testVersion), data.TargetFileMeta{}), "expected no error on adding already-installed version to library")

			// Confirm that there is nothing in the updates directory (no update performed)
			updateMatches, err := filepath.Glob(filepath.Join(testLibraryManager.updatesDirectory(binary), "*"))
			require.NoError(t, err, "error globbing for matches")
			require.Equal(t, 0, len(updateMatches), "expected no directories in updates directory but found: %+v", updateMatches)

			// Confirm that there is nothing in the staged updates directory (no update attempted)
			stagedUpdateMatches, err := filepath.Glob(filepath.Join(testLibraryManager.stagingDir, "*"))
			require.NoError(t, err, "error globbing for matches")
			require.Equal(t, 0, len(stagedUpdateMatches), "expected no directories in staged updates directory but found: %+v", stagedUpdateMatches)
		})
	}
}

func TestAddToLibrary_alreadyAdded(t *testing.T) {
	t.Parallel()

	for _, binary := range binaries {
		binary := binary
		t.Run(string(binary), func(t *testing.T) {
			t.Parallel()

			testBaseDir := t.TempDir()
			testMirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatalf("mirror should not have been called for download, but was: %s", r.URL.String())
			}))
			defer testMirror.Close()
			testLibraryManager := &updateLibraryManager{
				mirrorUrl:    testMirror.URL,
				mirrorClient: http.DefaultClient,
				logger:       log.NewNopLogger(),
				baseDir:      testBaseDir,
				stagingDir:   t.TempDir(),
				lock:         newLibraryLock(),
			}

			// Make sure our update directory exists
			require.NoError(t, os.MkdirAll(testLibraryManager.updatesDirectory(binary), 0755))

			// Ensure that a valid update already exists in that directory for the specified version
			testVersion := "2.2.2"
			executablePath := executableLocation(filepath.Join(testLibraryManager.updatesDirectory(binary), testVersion), binary)
			copyBinary(t, executablePath)
			require.NoError(t, os.Chmod(executablePath, 0755))
			_, err := os.Stat(executablePath)
			require.NoError(t, err, "did not create binary for test")
			require.NoError(t, autoupdate.CheckExecutable(context.TODO(), executablePath, "--version"), "binary created for test is corrupt")

			// Ask the library manager to perform the download
			targetFilename := fmt.Sprintf("%s-%s.tar.gz", binary, testVersion)
			require.Equal(t, testVersion, versionFromTarget(binary, targetFilename), "incorrectly formed target filename")
			require.NoError(t, testLibraryManager.AddToLibrary(binary, targetFilename, data.TargetFileMeta{}), "expected no error on adding already-downloaded version to library")

			// Confirm the requested version is still there
			_, err = os.Stat(executablePath)
			require.NoError(t, err, "could not stat update that should have existed")
		})
	}
}

func TestAddToLibrary_verifyStagedUpdate_handlesInvalidFiles(t *testing.T) {
	t.Parallel()

	// Set up TUF dependencies -- we do this here to avoid re-initializing the local tuf server for each
	// binary. It's unnecessary work since the mirror serves the same data both times.
	testBaseDir := t.TempDir()
	testReleaseVersion := "0.3.5"
	tufServerUrl, rootJson := initLocalTufServer(t, testReleaseVersion)
	metadataClient, err := initMetadataClient(testBaseDir, tufServerUrl, http.DefaultClient)
	require.NoError(t, err, "creating metadata client")
	// Re-initialize the metadata client with our test root JSON
	require.NoError(t, metadataClient.Init(rootJson), "could not initialize metadata client with test root JSON")
	_, err = metadataClient.Update()
	require.NoError(t, err, "could not update metadata client")

	// Get the target metadata
	launcherTargetMeta, err := metadataClient.Target(fmt.Sprintf("%s/%s/%s-%s.tar.gz", binaryLauncher, runtime.GOOS, binaryLauncher, testReleaseVersion))
	require.NoError(t, err, "could not get test metadata for launcher target")
	osquerydTargetMeta, err := metadataClient.Target(fmt.Sprintf("%s/%s/%s-%s.tar.gz", binaryOsqueryd, runtime.GOOS, binaryOsqueryd, testReleaseVersion))
	require.NoError(t, err, "could not get test metadata for launcher target")

	testCases := []struct {
		binary     autoupdatableBinary
		targetFile string
		targetMeta data.TargetFileMeta
	}{
		{
			binary:     binaryLauncher,
			targetFile: fmt.Sprintf("%s-%s.tar.gz", binaryLauncher, testReleaseVersion),
			targetMeta: launcherTargetMeta,
		},
		{
			binary:     binaryOsqueryd,
			targetFile: fmt.Sprintf("%s-%s.tar.gz", binaryOsqueryd, testReleaseVersion),
			targetMeta: osquerydTargetMeta,
		},
	}

	for _, tt := range testCases {
		tt := tt
		t.Run(string(tt.binary), func(t *testing.T) {
			t.Parallel()

			// Now, set up a mirror hosting an invalid file corresponding to our expected release
			invalidBinaryPath := filepath.Join(t.TempDir(), tt.targetFile)
			fh, err := os.Create(invalidBinaryPath)
			require.NoError(t, err, "could not create invalid binary for test")
			_, err = fh.Write([]byte("definitely not the executable we expect"))
			require.NoError(t, err, "could not write to invalid binary")
			fh.Close()
			testMaliciousMirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.ServeFile(w, r, invalidBinaryPath)
			}))
			defer testMaliciousMirror.Close()

			// Set up test library manager
			testLibraryManager, err := newUpdateLibraryManager(testMaliciousMirror.URL, http.DefaultClient, testBaseDir, log.NewNopLogger())
			require.NoError(t, err, "unexpected error creating new update library manager")

			// Request download
			require.Error(t, testLibraryManager.AddToLibrary(tt.binary, tt.targetFile, tt.targetMeta), "expected error when library manager downloads invalid file")

			// Confirm the update was removed after download
			downloadMatches, err := filepath.Glob(filepath.Join(testLibraryManager.stagingDir, "*"))
			require.NoError(t, err, "checking that staging dir did not have any downloads")
			require.Equal(t, 0, len(downloadMatches), "unexpected files found in staged updates directory: %+v", downloadMatches)

			// Confirm the update was not added to the library
			updateMatches, err := filepath.Glob(filepath.Join(testLibraryManager.updatesDirectory(tt.binary), "*"))
			require.NoError(t, err, "checking that updates directory does not contain any updates")
			require.Equal(t, 0, len(updateMatches), "unexpected files found in updates directory: %+v", updateMatches)
		})
	}
}

func TestTidyLibrary(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		testCaseName              string
		existingVersions          map[string]bool // maps versions to whether they're executable
		currentlyRunningVersion   string
		expectedPreservedVersions []string
		expectedRemovedVersions   []string
	}{
		{
			testCaseName: "more than 3 versions, currently running executable is within 3 newest",
			existingVersions: map[string]bool{
				"1.0.3":  true,
				"1.0.1":  true,
				"1.0.0":  true,
				"0.13.6": true,
				"0.12.4": true,
			},
			currentlyRunningVersion: "1.0.1",
			expectedPreservedVersions: []string{
				"1.0.3",
				"1.0.1",
				"1.0.0",
			},
			expectedRemovedVersions: []string{
				"0.13.6",
				"0.12.4",
			},
		},
		{
			testCaseName: "more than 3 versions, currently running executable is not within 3 newest",
			existingVersions: map[string]bool{
				"2.0.3":   true,
				"2.0.1":   true,
				"2.0.0":   true,
				"1.13.4":  true,
				"1.12.10": true,
			},
			currentlyRunningVersion: "1.12.10",
			expectedPreservedVersions: []string{
				"2.0.3",
				"2.0.1",
				"1.12.10",
			},
			expectedRemovedVersions: []string{
				"2.0.0",
				"1.13.4",
			},
		},
		{
			testCaseName: "more than 3 versions, currently running executable is not in update directory",
			existingVersions: map[string]bool{
				"0.20.3": true,
				"0.19.1": true,
				"0.17.0": true,
				"0.13.6": true,
				"0.12.4": true,
			},
			currentlyRunningVersion: "1.12.10",
			expectedPreservedVersions: []string{
				"0.20.3",
				"0.19.1",
			},
			expectedRemovedVersions: []string{
				"0.17.0",
				"0.13.6",
				"0.12.4",
			},
		},
		{
			testCaseName: "more than 3 versions, includes invalid semver",
			existingVersions: map[string]bool{
				"5.8.0":        true,
				"5.7.1":        true,
				"not_a_semver": true,
				"5.6.2":        true,
				"5.5.5":        true,
				"5.2.0":        true,
			},
			currentlyRunningVersion: "5.8.0",
			expectedPreservedVersions: []string{
				"5.8.0",
				"5.7.1",
				"5.6.2",
			},
			expectedRemovedVersions: []string{
				"not_a_semver",
				"5.5.5",
				"5.2.0",
			},
		},
		{
			testCaseName: "more than 3 versions, includes invalid executable within 3 newest",
			existingVersions: map[string]bool{
				"1.0.3":  true,
				"1.0.1":  false,
				"1.0.0":  true,
				"0.13.6": true,
				"0.12.4": true,
			},
			currentlyRunningVersion: "1.0.0",
			expectedPreservedVersions: []string{
				"1.0.3",
				"1.0.0",
				"0.13.6",
			},
			expectedRemovedVersions: []string{
				"1.0.1",
				"0.12.4",
			},
		},
		{
			testCaseName: "more than 3 versions, includes dev versions",
			existingVersions: map[string]bool{
				"1.0.3":             true,
				"1.0.3-9-g9c4a5ee":  true,
				"1.0.1":             true,
				"1.0.1-13-deadbeef": true,
				"1.0.0":             true,
			},
			currentlyRunningVersion: "1.0.1-13-deadbeef",
			expectedPreservedVersions: []string{
				"1.0.3",
				"1.0.3-9-g9c4a5ee",
				"1.0.1-13-deadbeef",
			},
			expectedRemovedVersions: []string{
				"1.0.1",
				"1.0.0",
			},
		},
		{
			testCaseName: "fewer than 3 versions",
			existingVersions: map[string]bool{
				"1.0.3": true,
				"1.0.1": true,
			},
			currentlyRunningVersion: "1.0.1",
			expectedPreservedVersions: []string{
				"1.0.3",
				"1.0.1",
			},
			expectedRemovedVersions: []string{},
		},
	}

	for _, binary := range binaries {
		binary := binary
		for _, tt := range testCases {
			tt := tt
			t.Run(string(binary)+": "+tt.testCaseName, func(t *testing.T) {
				t.Parallel()

				// Set up test library manager
				testBaseDir := t.TempDir()
				testLibraryManager := &updateLibraryManager{
					logger:     log.NewNopLogger(),
					baseDir:    testBaseDir,
					stagingDir: t.TempDir(),
					lock:       newLibraryLock(),
				}

				// Make a file in the staged updates directory
				f1, err := os.Create(filepath.Join(testLibraryManager.stagingDir, fmt.Sprintf("%s-1.2.3.tar.gz", binary)))
				require.NoError(t, err, "creating fake download file")
				f1.Close()

				// Confirm we made the files
				matches, err := filepath.Glob(filepath.Join(testLibraryManager.stagingDir, "*"))
				require.NoError(t, err, "could not glob for files in staged osqueryd download dir")
				require.Equal(t, 1, len(matches))

				// Set up existing versions for test
				for existingVersion, isExecutable := range tt.existingVersions {
					executablePath := executableLocation(filepath.Join(testLibraryManager.updatesDirectory(binary), existingVersion), binary)
					if !isExecutable && runtime.GOOS == "windows" {
						// We check file extension .exe to confirm executable on Windows, so trim the extension
						// if this test does not expect the file to be executable.
						executablePath = strings.TrimSuffix(executablePath, ".exe")
					}

					copyBinary(t, executablePath)

					if isExecutable {
						require.NoError(t, os.Chmod(executablePath, 0755))
					}
				}

				// Tidy the library
				testLibraryManager.TidyLibrary(binary, tt.currentlyRunningVersion)

				// Confirm the staging directory was tidied up
				_, err = os.Stat(testLibraryManager.stagingDir)
				require.NoError(t, err, "could not stat staged download dir")
				matchesAfter, err := filepath.Glob(filepath.Join(testLibraryManager.stagingDir, "*"))
				require.NoError(t, err, "could not glob for files in staged download dir")
				require.Equal(t, 0, len(matchesAfter))

				// Confirm that the versions we expect are still there
				for _, expectedPreservedVersion := range tt.expectedPreservedVersions {
					info, err := os.Stat(filepath.Join(testLibraryManager.updatesDirectory(binary), expectedPreservedVersion))
					require.NoError(t, err, "could not stat update dir that was expected to exist: %s", expectedPreservedVersion)
					require.True(t, info.IsDir())
				}

				// Confirm all other versions were removed
				for _, expectedRemovedVersion := range tt.expectedRemovedVersions {
					_, err := os.Stat(filepath.Join(testLibraryManager.updatesDirectory(binary), expectedRemovedVersion))
					require.Error(t, err, "expected version to be removed: %s", expectedRemovedVersion)
					require.True(t, os.IsNotExist(err))
				}
			})
		}
	}
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
	testLibrary, err := newUpdateLibraryManager("", nil, testBaseDir, log.NewNopLogger())
	require.NoError(t, err, "unexpected error creating new read-only library")

	// Get sorted versions
	validVersions, invalidVersions, err := testLibrary.sortedVersionsInLibrary(binaryLauncher)
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
			t.Parallel()

			// Create update directories
			testBaseDir := t.TempDir()

			// Set up test library
			testLibrary, err := newUpdateLibraryManager("", nil, testBaseDir, log.NewNopLogger())
			require.NoError(t, err, "unexpected error creating new read-only library")

			// Create cached version file
			expectedVersion := "5.5.5"
			require.NoError(t, os.WriteFile(
				filepath.Join(testLibrary.baseDir, fmt.Sprintf("%s-installed-version", binary)),
				[]byte(expectedVersion),
				0755))

			// Create fake executable in current working directory
			executablePath, err := os.Executable()
			require.NoError(t, err)
			testExecutablePath := filepath.Join(filepath.Dir(executablePath), string(binary))
			require.NoError(t, os.WriteFile(testExecutablePath, []byte("test"), 0755))
			t.Cleanup(func() {
				os.Remove(testExecutablePath)
			})

			actualVersion, actualPath, err := testLibrary.installedVersion(binary)
			require.NoError(t, err, "could not get installed version")
			require.NotNil(t, actualVersion)
			require.NotEqual(t, "", actualPath)
		})
	}
}

func Test_cacheInstalledVersion(t *testing.T) {
	t.Parallel()

	// Create update directories
	testBaseDir := t.TempDir()

	// Set up test library
	testLibrary, err := newUpdateLibraryManager("", nil, testBaseDir, log.NewNopLogger())
	require.NoError(t, err, "unexpected error creating new library")

	versionToCache, err := semver.NewVersion("1.2.3-45-abcdabcd")
	require.NoError(t, err, "unexpected error parsing semver")

	for _, binary := range binaries {
		// Confirm cache file doesn't exist yet
		expectedCacheFileLocation := testLibrary.cachedInstalledVersionLocation(binary)
		_, err = os.Stat(expectedCacheFileLocation)
		require.True(t, os.IsNotExist(err), "cache file exists but should not have been created yet")

		// Cache it
		testLibrary.cacheInstalledVersion(binary, versionToCache)

		// Confirm cache file exists
		_, err = os.Stat(expectedCacheFileLocation)
		require.NoError(t, err, "cache file %s does not exist but should have been created", expectedCacheFileLocation)

		// Compare versions
		cachedVersion, err := testLibrary.getCachedInstalledVersion(binary)
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

func Test_parseOsquerydVersion_Windows(t *testing.T) {
	t.Parallel()

	osquerydVersionOutput := `osqueryd.exe version 5.8.2`

	v, err := parseOsquerydVersion([]byte(osquerydVersionOutput))
	require.NoError(t, err, "should be able to parse osqueryd version without error")
	require.NotNil(t, v, "should have been able to parse osqueryd version as semver")
	require.Equal(t, "5.8.2", v.Original(), "osqueryd semver should match")
}

func copyBinary(t *testing.T, executablePath string) {
	require.NoError(t, os.MkdirAll(filepath.Dir(executablePath), 0755))

	destFile, err := os.Create(executablePath)
	require.NoError(t, err, "create destination file")
	defer destFile.Close()

	srcFile, err := os.Open(os.Args[0])
	require.NoError(t, err, "opening binary to copy for test")
	defer srcFile.Close()

	_, err = io.Copy(destFile, srcFile)
	require.NoError(t, err, "copying binary")
}
