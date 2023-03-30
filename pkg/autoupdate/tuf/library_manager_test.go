package tuf

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Masterminds/semver"
	"github.com/go-kit/kit/log"
	localservermocks "github.com/kolide/launcher/ee/localserver/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_newUpdateLibraryManager(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()
	_, err := newUpdateLibraryManager(nil, "", nil, testRootDir, runtime.GOOS, nil, log.NewNopLogger())
	require.NoError(t, err, "unexpected error creating new update library manager")

	stagedOsquerydDownloadDir, err := os.Stat(filepath.Join(testRootDir, "osqueryd-staged-updates"))
	require.NoError(t, err, "could not stat staged osqueryd download dir")
	require.True(t, stagedOsquerydDownloadDir.IsDir(), "staged osqueryd download dir is not a directory")

	osquerydDownloadDir, err := os.Stat(filepath.Join(testRootDir, "osqueryd-updates"))
	require.NoError(t, err, "could not stat osqueryd download dir")
	require.True(t, osquerydDownloadDir.IsDir(), "osqueryd download dir is not a directory")

	stagedLauncherDownloadDir, err := os.Stat(filepath.Join(testRootDir, "launcher-staged-updates"))
	require.NoError(t, err, "could not stat staged launcher download dir")
	require.True(t, stagedLauncherDownloadDir.IsDir(), "staged launcher download dir is not a directory")

	launcherDownloadDir, err := os.Stat(filepath.Join(testRootDir, "launcher-updates"))
	require.NoError(t, err, "could not stat launcher download dir")
	require.True(t, launcherDownloadDir.IsDir(), "launcher download dir is not a directory")
}

func Test_addToLibrary(t *testing.T) {
	t.Parallel()

	for _, binary := range binaries {
		binary := binary
		t.Run(binary, func(t *testing.T) {
			t.Parallel()

			// Set up TUF dependencies
			testRootDir := t.TempDir()
			testReleaseVersion := "1.2.3"
			tufServerUrl, rootJson := initLocalTufServer(t, testReleaseVersion)
			metadataClient, err := initMetadataClient(testRootDir, tufServerUrl, http.DefaultClient)
			require.NoError(t, err, "creating metadata client")
			// Re-initialize the metadata client with our test root JSON
			require.NoError(t, metadataClient.Init(rootJson), "could not initialize metadata client with test root JSON")
			_, err = metadataClient.Update()
			require.NoError(t, err, "could not update metadata client")

			// Set up test library manager
			mockOsquerier := localservermocks.NewQuerier(t)
			testLibraryManager, err := newUpdateLibraryManager(metadataClient, tufServerUrl, http.DefaultClient, testRootDir, runtime.GOOS, mockOsquerier, log.NewNopLogger())
			require.NoError(t, err, "unexpected error creating new update library manager")

			// For osqueryd, make sure we check that the running version is not equal to the target version
			if binary == "osqueryd" {
				mockOsquerier.On("Query", mock.Anything).Return([]map[string]string{{"version": "5.5.5"}}, nil).Once()
			}

			// Request download
			require.NoError(t, testLibraryManager.AddToLibrary(binary, fmt.Sprintf("%s-%s.tar.gz", binary, testReleaseVersion)), "expected no error adding to library")
			mockOsquerier.AssertExpectations(t)

			// Confirm the update was downloaded
			dirInfo, err := os.Stat(filepath.Join(testLibraryManager.updatesDirectory(binary), testReleaseVersion))
			require.NoError(t, err, "checking that update was downloaded")
			require.True(t, dirInfo.IsDir())
			executableInfo, err := os.Stat(executableLocation(filepath.Join(testLibraryManager.updatesDirectory(binary), testReleaseVersion), binary))
			require.NoError(t, err, "checking that downloaded update includes executable")
			require.False(t, executableInfo.IsDir())

			// Confirm the staging directory is empty
			matches, err := filepath.Glob(filepath.Join(testLibraryManager.stagedUpdatesDirectory(binary), "*"))
			require.NoError(t, err, "checking that staging dir was cleaned")
			require.Equal(t, 0, len(matches), "unexpected files found in staged updates directory: %+v", matches)
		})
	}
}

func Test_addToLibrary_alreadyRunning_osqueryd(t *testing.T) {
	t.Parallel()

	// We only do this particular test for osqueryd because in test, version.Version()
	// returns `unknown` for everything when we attempt to get the current running version
	// of launcher, which is not something that the semver library can parse.

	testRootDir := t.TempDir()
	mockOsquerier := localservermocks.NewQuerier(t)
	testLibraryManager := &updateLibraryManager{
		logger:        log.NewNopLogger(),
		rootDirectory: testRootDir,
		osquerier:     mockOsquerier,
	}

	// Make sure our update directories exist so we can verify they're empty later
	require.NoError(t, os.MkdirAll(testLibraryManager.updatesDirectory("osqueryd"), 0755))
	require.NoError(t, os.MkdirAll(testLibraryManager.stagedUpdatesDirectory("osqueryd"), 0755))

	// Set osquerier to return same version we want to add to the library
	testVersion := "0.12.1-abcdabcd"
	expectedOsqueryVersion, err := semver.NewVersion(testVersion)
	require.NoError(t, err)

	// Expect to return one row containing the version
	mockOsquerier.On("Query", mock.Anything).Return([]map[string]string{{"version": expectedOsqueryVersion.Original()}}, nil).Once()

	// Ask the library manager to perform the download
	require.NoError(t, testLibraryManager.AddToLibrary("osqueryd", fmt.Sprintf("osqueryd-%s.tar.gz", testVersion)), "expected no error on adding already-running version to library")
	mockOsquerier.AssertExpectations(t)

	// Confirm that there is nothing in the updates directory (no update performed)
	updateMatches, err := filepath.Glob(filepath.Join(testLibraryManager.updatesDirectory("osqueryd"), "*"))
	require.NoError(t, err, "error globbing for matches")
	require.Equal(t, 0, len(updateMatches), "expected no directories in updates directory but found: %+v", updateMatches)

	// Confirm that there is nothing in the staged updates directory (no update attempted)
	stagedUpdateMatches, err := filepath.Glob(filepath.Join(testLibraryManager.stagedUpdatesDirectory("osqueryd"), "*"))
	require.NoError(t, err, "error globbing for matches")
	require.Equal(t, 0, len(stagedUpdateMatches), "expected no directories in staged updates directory but found: %+v", stagedUpdateMatches)
}

func Test_addToLibrary_alreadyAdded(t *testing.T) {
	t.Parallel()

	for _, binary := range binaries {
		binary := binary
		t.Run(binary, func(t *testing.T) {
			t.Parallel()

			testRootDir := t.TempDir()
			mockOsquerier := localservermocks.NewQuerier(t)
			testLibraryManager := &updateLibraryManager{
				logger:        log.NewNopLogger(),
				rootDirectory: testRootDir,
				osquerier:     mockOsquerier,
			}

			// Make sure our update directory exists
			require.NoError(t, os.MkdirAll(testLibraryManager.updatesDirectory(binary), 0755))

			// Ensure that a valid update already exists in that directory for the specified version
			testVersion := "2.2.2"
			executablePath := executableLocation(filepath.Join(testLibraryManager.updatesDirectory(binary), testVersion), binary)
			copyBinary(t, executablePath)
			require.NoError(t, os.Chmod(executablePath, 0755))

			// For osqueryd, make sure we check that the running version is not equal to the target version
			if binary == "osqueryd" {
				mockOsquerier.On("Query", mock.Anything).Return([]map[string]string{{"version": "5.5.5"}}, nil).Once()
			}

			// Ask the library manager to perform the download
			require.NoError(t, testLibraryManager.AddToLibrary(binary, fmt.Sprintf("%s-%s.tar.gz", binary, testVersion)), "expected no error on adding already-downloaded version to library")
			mockOsquerier.AssertExpectations(t)

			// Confirm the requested version is still there
			_, err := os.Stat(executablePath)
			require.NoError(t, err, "could not stat update that should have existed")
		})
	}
}

func Test_addToLibrary_verifyStagedUpdate_handlesInvalidFiles(t *testing.T) {
	t.Parallel()
	t.Skip("TODO")
}

func Test_currentRunningVersion_launcher_errorWhenVersionIsNotSet(t *testing.T) {
	t.Parallel()

	testLibraryManager := &updateLibraryManager{
		logger: log.NewNopLogger(),
	}

	// In test, version.Version() returns `unknown` for everything, which is not something
	// that the semver library can parse. So we only expect an error here.
	launcherVersion, err := testLibraryManager.currentRunningVersion("launcher")
	require.Error(t, err, "expected an error fetching current running version of launcher")
	require.Nil(t, launcherVersion)
}

func Test_currentRunningVersion_osqueryd(t *testing.T) {
	t.Parallel()

	mockOsquerier := localservermocks.NewQuerier(t)

	testLibraryManager := &updateLibraryManager{
		logger:    log.NewNopLogger(),
		osquerier: mockOsquerier,
	}

	expectedOsqueryVersion, err := semver.NewVersion("5.10.12")
	require.NoError(t, err)

	// Expect to return one row containing the version
	mockOsquerier.On("Query", mock.Anything).Return([]map[string]string{{"version": expectedOsqueryVersion.Original()}}, nil).Once()

	osqueryVersion, err := testLibraryManager.currentRunningVersion("osqueryd")
	require.NoError(t, err, "expected no error fetching current running version of osqueryd")
	require.Equal(t, expectedOsqueryVersion.Original(), osqueryVersion.Original())
}

func Test_currentRunningVersion_osqueryd_handlesQueryError(t *testing.T) {
	t.Parallel()

	mockOsquerier := localservermocks.NewQuerier(t)

	testLibraryManager := &updateLibraryManager{
		logger:    log.NewNopLogger(),
		osquerier: mockOsquerier,
	}

	// Expect to return an error
	mockOsquerier.On("Query", mock.Anything).Return(make([]map[string]string, 0), errors.New("test osqueryd querying error")).Once()

	osqueryVersion, err := testLibraryManager.currentRunningVersion("osqueryd")
	require.Error(t, err, "expected an error returning osquery version when querying osquery fails")
	require.Nil(t, osqueryVersion)
}

func Test_tidyLibrary(t *testing.T) {
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
			t.Run(binary+": "+tt.testCaseName, func(t *testing.T) {
				t.Parallel()

				// Set up test library manager
				rootDir := t.TempDir()
				testLibraryManager := &updateLibraryManager{
					logger:        log.NewNopLogger(),
					rootDirectory: rootDir,
				}

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

				// Prepare the current version
				currentVersion, err := semver.NewVersion(tt.currentlyRunningVersion)
				require.NoError(t, err, "invalid current version for test: %s", tt.currentlyRunningVersion)

				// Tidy the library
				testLibraryManager.tidyLibrary(binary, currentVersion)

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

func Test_executableLocation(t *testing.T) {
	t.Parallel()

	updateDir := filepath.Join("some", "path", "to", "the", "updates", "directory")

	var expectedOsquerydLocation string
	var expectedLauncherLocation string
	switch runtime.GOOS {
	case "darwin":
		expectedOsquerydLocation = filepath.Join(updateDir, "osqueryd")
		expectedLauncherLocation = filepath.Join(updateDir, "Kolide.app", "Contents", "MacOS", "launcher")
	case "windows":
		expectedOsquerydLocation = filepath.Join(updateDir, "osqueryd.exe")
		expectedLauncherLocation = filepath.Join(updateDir, "launcher.exe")
	case "linux":
		expectedOsquerydLocation = filepath.Join(updateDir, "osqueryd")
		expectedLauncherLocation = filepath.Join(updateDir, "launcher")
	}

	osquerydLocation := executableLocation(updateDir, "osqueryd")
	require.Equal(t, expectedOsquerydLocation, osquerydLocation)

	launcherLocation := executableLocation(updateDir, "launcher")
	require.Equal(t, expectedLauncherLocation, launcherLocation)
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
