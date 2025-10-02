package tuf

import (
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/kolide/kit/ulid"
	tufci "github.com/kolide/launcher/ee/tuf/ci"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
	"github.com/theupdateframework/go-tuf/data"
)

func Test_newUpdateLibraryManager(t *testing.T) {
	t.Parallel()

	testBaseDir := filepath.Join(t.TempDir(), "updates")
	testLibraryManager, err := newUpdateLibraryManager("", nil, testBaseDir, multislogger.NewNopLogger())
	require.NoError(t, err, "unexpected error creating new update library manager")

	baseDir, err := os.Stat(testBaseDir)
	require.NoError(t, err, "could not stat base dir")
	require.True(t, baseDir.IsDir(), "base dir is not a directory")

	osquerydDownloadDir, err := os.Stat(updatesDirectory("osqueryd", testLibraryManager.baseDir))
	require.NoError(t, err, "could not stat osqueryd download dir")
	require.True(t, osquerydDownloadDir.IsDir(), "osqueryd download dir is not a directory")

	launcherDownloadDir, err := os.Stat(updatesDirectory("launcher", testLibraryManager.baseDir))
	require.NoError(t, err, "could not stat launcher download dir")
	require.True(t, launcherDownloadDir.IsDir(), "launcher download dir is not a directory")
}

func Test_pathToTargetVersionExecutable(t *testing.T) {
	t.Parallel()

	testBaseDir := DefaultLibraryDirectory(t.TempDir())

	testVersion := "1.0.7-30-abcdabcd"
	testTargetFilename := fmt.Sprintf("launcher-%s.tar.gz", testVersion)
	expectedPath := filepath.Join(testBaseDir, "launcher", testVersion, "launcher")
	if runtime.GOOS == "darwin" {
		expectedPath = filepath.Join(testBaseDir, "launcher", testVersion, "Kolide.app", "Contents", "MacOS", "launcher")
	} else if runtime.GOOS == "windows" {
		expectedPath = expectedPath + ".exe"
	}

	actualPath, actualVersion := pathToTargetVersionExecutable(binaryLauncher, testTargetFilename, testBaseDir)
	require.Equal(t, expectedPath, actualPath, "path mismatch")
	require.Equal(t, testVersion, actualVersion, "version mismatch")
}

func TestAvailable(t *testing.T) {
	t.Parallel()

	// Create update directories
	testBaseDir := t.TempDir()

	// Set up test library
	testLibrary, err := newUpdateLibraryManager("", nil, testBaseDir, multislogger.NewNopLogger())
	require.NoError(t, err, "unexpected error creating new read-only library")

	// Set up valid "osquery" executable
	runningOsqueryVersion := "5.5.7"
	runningTarget := fmt.Sprintf("osqueryd-%s.tar.gz", runningOsqueryVersion)
	executablePath, _ := pathToTargetVersionExecutable(binaryOsqueryd, runningTarget, testBaseDir)
	tufci.CopyBinary(t, executablePath)
	require.NoError(t, os.Chmod(executablePath, 0755))

	// Query for the current osquery version
	require.True(t, testLibrary.Available(binaryOsqueryd, runningTarget))

	// Query for a different osqueryd version
	require.False(t, testLibrary.Available(binaryOsqueryd, "osqueryd-5.6.7.tar.gz"))
}

func TestAddToLibrary(t *testing.T) {
	t.Parallel()

	for _, b := range []autoupdatableBinary{binaryLauncher, binaryOsqueryd} {
		b := b
		t.Run(string(b), func(t *testing.T) {
			t.Parallel()

			// Set up TUF dependencies
			testBaseDir := t.TempDir()
			testReleaseVersion := "1.2.4"
			tufServerUrl, rootJson := tufci.InitRemoteTufServer(t, testReleaseVersion)
			metadataClient, err := initMetadataClient(t.Context(), testBaseDir, tufServerUrl, http.DefaultClient)
			require.NoError(t, err, "creating metadata client")

			// Re-initialize the metadata client with our test root JSON
			require.NoError(t, metadataClient.Init(rootJson), "could not initialize metadata client with test root JSON")
			_, err = metadataClient.Update()
			require.NoError(t, err, "could not update metadata client")

			// Get the target metadata
			targetMeta, err := metadataClient.Target(fmt.Sprintf("%s/%s/%s/%s-%s.tar.gz", b, runtime.GOOS, PlatformArch(), b, testReleaseVersion))
			require.NoError(t, err, "could not get test metadata for target")

			targetFile := fmt.Sprintf("%s-%s.tar.gz", b, testReleaseVersion)

			// Set up test library manager
			testLibraryManager, err := newUpdateLibraryManager(tufServerUrl, http.DefaultClient, testBaseDir, multislogger.NewNopLogger())
			require.NoError(t, err, "unexpected error creating new update library manager")

			// Request download -- make a couple concurrent requests to confirm that the lock works.
			var wg sync.WaitGroup
			for i := 0; i < 5; i += 1 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					require.NoError(t, testLibraryManager.AddToLibrary(b, "", targetFile, targetMeta), "expected no error adding to library")
				}()
			}

			wg.Wait()

			// Confirm the update was downloaded
			dirInfo, err := os.Stat(filepath.Join(updatesDirectory(b, testBaseDir), testReleaseVersion))
			require.NoError(t, err, "checking that update was downloaded")
			require.True(t, dirInfo.IsDir())
			if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
				require.Equal(t, "drwxr-xr-x", dirInfo.Mode().String())
			}
			executableInfo, err := os.Stat(executableLocation(filepath.Join(updatesDirectory(b, testBaseDir), testReleaseVersion), b))
			require.NoError(t, err, "checking that downloaded update includes executable")
			require.False(t, executableInfo.IsDir())
		})
	}
}

func TestAddToLibrary_alreadyRunning(t *testing.T) {
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
				slogger:      multislogger.NewNopLogger(),
				baseDir:      testBaseDir,
				lock:         newLibraryLock(),
			}

			// Make sure our update directory exists
			require.NoError(t, os.MkdirAll(updatesDirectory(binary, testBaseDir), 0755))

			// Set the current running version to the version we're going to request to download
			currentRunningVersion := "4.3.2"

			// Ask the library manager to perform the download
			targetFilename := fmt.Sprintf("%s-%s.tar.gz", binary, currentRunningVersion)
			require.Equal(t, currentRunningVersion, versionFromTarget(binary, targetFilename), "incorrectly formed target filename")
			require.NoError(t, testLibraryManager.AddToLibrary(binary, currentRunningVersion, targetFilename, data.TargetFileMeta{}), "expected no error on adding already-downloaded version to library")

			// Confirm the requested version was not downloaded
			_, err := os.Stat(filepath.Join(updatesDirectory(binary, testBaseDir), currentRunningVersion))
			require.True(t, os.IsNotExist(err), "should not have downloaded currently-running version")
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
				slogger:      multislogger.NewNopLogger(),
				baseDir:      testBaseDir,
				lock:         newLibraryLock(),
			}

			// Make sure our update directory exists
			require.NoError(t, os.MkdirAll(updatesDirectory(binary, testBaseDir), 0755))

			// Ensure that a valid update already exists in that directory for the specified version
			testVersion := "2.2.2"
			executablePath := executableLocation(filepath.Join(updatesDirectory(binary, testBaseDir), testVersion), binary)
			tufci.CopyBinary(t, executablePath)
			require.NoError(t, os.Chmod(executablePath, 0755))
			_, err := os.Stat(executablePath)
			require.NoError(t, err, "did not create binary for test")
			require.NoError(t, CheckExecutable(t.Context(), multislogger.NewNopLogger(), executablePath, "--version"), "binary created for test is corrupt")

			// Ask the library manager to perform the download
			targetFilename := fmt.Sprintf("%s-%s.tar.gz", binary, testVersion)
			require.Equal(t, testVersion, versionFromTarget(binary, targetFilename), "incorrectly formed target filename")
			require.NoError(t, testLibraryManager.AddToLibrary(binary, "", targetFilename, data.TargetFileMeta{}), "expected no error on adding already-downloaded version to library")

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
	tufServerUrl, rootJson := tufci.InitRemoteTufServer(t, testReleaseVersion)
	metadataClient, err := initMetadataClient(t.Context(), testBaseDir, tufServerUrl, http.DefaultClient)
	require.NoError(t, err, "creating metadata client")
	// Re-initialize the metadata client with our test root JSON
	require.NoError(t, metadataClient.Init(rootJson), "could not initialize metadata client with test root JSON")
	_, err = metadataClient.Update()
	require.NoError(t, err, "could not update metadata client")

	// Get the target metadata
	launcherTargetMeta, err := metadataClient.Target(fmt.Sprintf("%s/%s/%s/%s-%s.tar.gz", binaryLauncher, runtime.GOOS, PlatformArch(), binaryLauncher, testReleaseVersion))
	require.NoError(t, err, "could not get test metadata for launcher target")
	osquerydTargetMeta, err := metadataClient.Target(fmt.Sprintf("%s/%s/%s/%s-%s.tar.gz", binaryOsqueryd, runtime.GOOS, PlatformArch(), binaryOsqueryd, testReleaseVersion))
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
			testLibraryManager, err := newUpdateLibraryManager(testMaliciousMirror.URL, http.DefaultClient, testBaseDir, multislogger.NewNopLogger())
			require.NoError(t, err, "unexpected error creating new update library manager")

			// Request download
			require.Error(t, testLibraryManager.AddToLibrary(tt.binary, "", tt.targetFile, tt.targetMeta), "expected error when library manager downloads invalid file")

			// Confirm the update was not added to the library
			updateMatches, err := filepath.Glob(filepath.Join(updatesDirectory(tt.binary, testBaseDir), "*"))
			require.NoError(t, err, "checking that updates directory does not contain any updates")
			require.Equal(t, 0, len(updateMatches), "unexpected files found in updates directory: %+v", updateMatches)
		})
	}
}

func Test_sanitizeExtractPath(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		filepath    string
		destination string
		expectError bool
	}{
		{
			filepath:    "file",
			destination: "/tmp",
			expectError: false,
		},
		{
			filepath:    "subdir/../subdir/file",
			destination: "/tmp",
			expectError: false,
		},

		{
			filepath:    "../../../file",
			destination: "/tmp",
			expectError: true,
		},
		{
			filepath:    "./././file",
			destination: "/tmp",
			expectError: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.filepath, func(t *testing.T) {
			t.Parallel()

			if tt.expectError {
				require.Error(t, sanitizeExtractPath(tt.filepath, tt.destination), tt.filepath)
			} else {
				require.NoError(t, sanitizeExtractPath(tt.filepath, tt.destination), tt.filepath)
			}
		})
	}
}

func Test_sanitizePermissions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		testCaseName         string
		givenFilePermissions fs.FileMode
	}{
		{
			testCaseName:         "directory, valid permissions",
			givenFilePermissions: fs.ModeDir | 0755,
		},
		{
			testCaseName:         "directory, invalid permissions (group has write)",
			givenFilePermissions: fs.ModeDir | 0775,
		},
		{
			testCaseName:         "directory, invalid permissions (public has write)",
			givenFilePermissions: fs.ModeDir | 0757,
		},
		{
			testCaseName:         "directory, invalid permissions (everyone has write)",
			givenFilePermissions: fs.ModeDir | 0777,
		},
		{
			testCaseName:         "executable file, valid permissions",
			givenFilePermissions: 0755,
		},
		{
			testCaseName:         "executable file, invalid permissions (group has write)",
			givenFilePermissions: 0775,
		},
		{
			testCaseName:         "executable file, invalid permissions (public has write)",
			givenFilePermissions: 0757,
		},
		{
			testCaseName:         "executable file, invalid permissions (everyone has write)",
			givenFilePermissions: 0777,
		},
		{
			testCaseName:         "non-executable file, valid permissions",
			givenFilePermissions: 0644,
		},
		{
			testCaseName:         "non-executable file, invalid permissions (group has write)",
			givenFilePermissions: 0664,
		},
		{
			testCaseName:         "non-executable file, invalid permissions (public has write)",
			givenFilePermissions: 0646,
		},
		{
			testCaseName:         "non-executable file, invalid permissions (everyone has write)",
			givenFilePermissions: 0666,
		},
	}

	for _, tt := range testCases {
		tt := tt
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			// Create a temp file to extract a FileInfo from it with tt.givenFilePermissions
			tmpDir := t.TempDir()
			pathUnderTest := filepath.Join(tmpDir, ulid.New())
			if tt.givenFilePermissions.IsDir() {
				require.NoError(t, os.MkdirAll(pathUnderTest, tt.givenFilePermissions))
			} else {
				f, err := os.OpenFile(pathUnderTest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, tt.givenFilePermissions)
				require.NoError(t, err)
				require.NoError(t, f.Close())
			}
			fileInfo, err := os.Stat(pathUnderTest)
			require.NoError(t, err)

			sanitizedPermissions := sanitizePermissions(fileInfo)

			// Confirm no group write
			require.True(t, sanitizedPermissions&0020 == 0)

			// Confirm no public write
			require.True(t, sanitizedPermissions&0002 == 0)

			// Confirm type is set correctly
			require.Equal(t, tt.givenFilePermissions.Type(), sanitizedPermissions.Type())

			// Confirm owner permissions are unmodified
			var ownerBits fs.FileMode = 0700
			if runtime.GOOS == "windows" {
				// Windows doesn't have executable bit
				ownerBits = 0600
			}
			require.Equal(t, tt.givenFilePermissions&ownerBits, sanitizedPermissions&ownerBits)
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
			testCaseName: "more than 2 versions, currently running executable is within 2 newest",
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
			},
			expectedRemovedVersions: []string{
				"1.0.0",
				"0.13.6",
				"0.12.4",
			},
		},
		{
			testCaseName: "more than 2 versions, currently running executable is not within 2 newest",
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
				"1.12.10",
			},
			expectedRemovedVersions: []string{
				"2.0.1",
				"2.0.0",
				"1.13.4",
			},
		},
		{
			testCaseName: "more than 2 versions, currently running executable is not in update directory",
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
			},
			expectedRemovedVersions: []string{
				"0.19.1",
				"0.17.0",
				"0.13.6",
				"0.12.4",
			},
		},
		{
			testCaseName: "more than 2 versions, includes invalid semver",
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
			},
			expectedRemovedVersions: []string{
				"not_a_semver",
				"5.6.2",
				"5.5.5",
				"5.2.0",
			},
		},
		{
			testCaseName: "more than 2 versions, includes invalid executable within 2 newest",
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
			},
			expectedRemovedVersions: []string{
				"1.0.1",
				"0.13.6",
				"0.12.4",
			},
		},
		{
			testCaseName: "more than 2 versions, includes dev versions",
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
				"1.0.1-13-deadbeef",
			},
			expectedRemovedVersions: []string{
				"1.0.3-9-g9c4a5ee",
				"1.0.1",
				"1.0.0",
			},
		},
		{
			testCaseName: "fewer than 2 versions",
			existingVersions: map[string]bool{
				"1.0.3": true,
			},
			currentlyRunningVersion: "1.0.3",
			expectedPreservedVersions: []string{
				"1.0.3",
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
					slogger: multislogger.NewNopLogger(),
					baseDir: testBaseDir,
					lock:    newLibraryLock(),
				}

				// Set up existing versions for test
				for existingVersion, isExecutable := range tt.existingVersions {
					executablePath := executableLocation(filepath.Join(updatesDirectory(binary, testBaseDir), existingVersion), binary)
					if !isExecutable && runtime.GOOS == "windows" {
						// We check file extension .exe to confirm executable on Windows, so trim the extension
						// if this test does not expect the file to be executable.
						executablePath = strings.TrimSuffix(executablePath, ".exe")
					}

					if isExecutable {
						tufci.CopyBinary(t, executablePath)
						require.NoError(t, os.Chmod(executablePath, 0755))
					} else {
						// Create a non-executable file at `executablePath`
						require.NoError(t, os.MkdirAll(filepath.Dir(executablePath), 0755))
						f, err := os.Create(executablePath)
						require.NoError(t, err, "creating non-executable file for test")
						_, err = f.Write([]byte("test"))
						require.NoError(t, err, "writing non-executable file for test")
						f.Close()
					}
				}

				// Confirm we made the update files
				updateMatches, err := filepath.Glob(filepath.Join(updatesDirectory(binary, testBaseDir), "*"))
				require.NoError(t, err, "could not glob for directories in updates dir")
				require.Equal(t, len(tt.existingVersions), len(updateMatches))

				// Tidy the library
				testLibraryManager.TidyLibrary(binary, tt.currentlyRunningVersion)

				// Confirm that the versions we expect are still there
				for _, expectedPreservedVersion := range tt.expectedPreservedVersions {
					info, err := os.Stat(filepath.Join(updatesDirectory(binary, testBaseDir), expectedPreservedVersion))
					require.NoError(t, err, "could not stat update dir that was expected to exist: %s", expectedPreservedVersion)
					require.True(t, info.IsDir())
				}

				// Confirm all other versions were removed
				for _, expectedRemovedVersion := range tt.expectedRemovedVersions {
					_, err := os.Stat(filepath.Join(updatesDirectory(binary, testBaseDir), expectedRemovedVersion))
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

	// Create a few valid updates in the library
	olderValidVersion := "0.13.5"
	middleValidVersion := "1.0.7-11-abcdabcd"
	secondMiddleValidVersion := "1.0.7-16-g6e6704e1dc33"
	newerValidVersion := "1.0.7"
	for _, v := range []string{olderValidVersion, middleValidVersion, secondMiddleValidVersion, newerValidVersion} {
		versionDir := filepath.Join(testBaseDir, "launcher", v)
		executablePath := executableLocation(versionDir, binaryLauncher)
		require.NoError(t, os.MkdirAll(filepath.Dir(executablePath), 0755))
		tufci.CopyBinary(t, executablePath)
		require.NoError(t, os.Chmod(executablePath, 0755))
		_, err := os.Stat(executablePath)
		require.NoError(t, err, "did not create binary for test")
		require.NoError(t, CheckExecutable(t.Context(), multislogger.NewNopLogger(), executablePath, "--version"), "binary created for test is corrupt")
	}

	// Get sorted versions
	validVersions, invalidVersions, err := sortedVersionsInLibrary(t.Context(), multislogger.NewNopLogger(), binaryLauncher, testBaseDir)
	require.NoError(t, err, "expected no error on sorting versions in library")

	// Confirm invalid versions are the ones we expect
	require.Equal(t, 2, len(invalidVersions))
	require.Contains(t, invalidVersions, invalidVersion)
	require.Contains(t, invalidVersions, corruptedVersion)

	// Confirm valid versions are the ones we expect and that they're sorted in ascending order
	require.Equal(t, 4, len(validVersions))
	require.Equal(t, olderValidVersion, validVersions[0], "not sorted")
	require.Equal(t, middleValidVersion, validVersions[1], "not sorted")
	require.Equal(t, secondMiddleValidVersion, validVersions[2], "not sorted")
	require.Equal(t, newerValidVersion, validVersions[3], "not sorted")
}

func Test_sortedVersionsInLibrary_devBuilds(t *testing.T) {
	t.Parallel()

	// Create update directories
	testBaseDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(testBaseDir, "launcher"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(testBaseDir, "osqueryd"), 0755))

	// Create a few valid updates in the library
	olderVersion := "1.0.16-8-g1594781"
	middleVersion := "1.0.16-9-g613d85c"
	newerVersion := "1.0.16-30-ge34c9a0"
	for _, v := range []string{olderVersion, middleVersion, newerVersion} {
		versionDir := filepath.Join(testBaseDir, "launcher", v)
		executablePath := executableLocation(versionDir, binaryLauncher)
		require.NoError(t, os.MkdirAll(filepath.Dir(executablePath), 0755))
		tufci.CopyBinary(t, executablePath)
		require.NoError(t, os.Chmod(executablePath, 0755))
		_, err := os.Stat(executablePath)
		require.NoError(t, err, "did not create binary for test")
		require.NoError(t, CheckExecutable(t.Context(), multislogger.NewNopLogger(), executablePath, "--version"), "binary created for test is corrupt")
	}

	// Get sorted versions
	validVersions, invalidVersions, err := sortedVersionsInLibrary(t.Context(), multislogger.NewNopLogger(), binaryLauncher, testBaseDir)
	require.NoError(t, err, "expected no error on sorting versions in library")

	// Confirm we don't have any invalid versions
	require.Equal(t, 0, len(invalidVersions))

	// Confirm valid versions are the ones we expect and that they're sorted in ascending order
	require.Equal(t, 3, len(validVersions))
	require.Equal(t, olderVersion, validVersions[0], "not sorted")
	require.Equal(t, middleVersion, validVersions[1], "not sorted")
	require.Equal(t, newerVersion, validVersions[2], "not sorted")
}
