package tuf

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	localservermocks "github.com/kolide/launcher/ee/localserver/mocks"
	"github.com/kolide/launcher/pkg/agent/storage"
	storageci "github.com/kolide/launcher/pkg/agent/storage/ci"
	"github.com/kolide/launcher/pkg/agent/types"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
	"github.com/theupdateframework/go-tuf"
)

func TestNewTufAutoupdater(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()
	s := setupStorage(t)

	_, err := NewTufAutoupdater("https://example.com", testRootDir, "", http.DefaultClient, "https://example.com", http.DefaultClient, s, localservermocks.NewQuerier(t))
	require.NoError(t, err, "could not initialize new TUF autoupdater")

	// Confirm that the TUF directory we expose is the one that we created
	exposedRootDir := LocalTufDirectory(testRootDir)

	_, err = os.Stat(exposedRootDir)
	require.NoError(t, err, "could not stat TUF directory that should have been initialized in test")

	_, err = os.Stat(filepath.Join(exposedRootDir, "root.json"))
	require.NoError(t, err, "could not stat root.json that should have been created in test")

	// Confirm that the library manager's base directory was set correctly
	_, err = os.Stat(filepath.Join(testRootDir, "updates"))
	require.NoError(t, err, "could not stat updates directory that should have been created for library manager")
}

// Tests running as well as shutdown
func TestExecute(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()
	testReleaseVersion := "1.2.3"
	tufServerUrl, rootJson := initLocalTufServer(t, testReleaseVersion)
	s := setupStorage(t)

	// Set up autoupdater
	autoupdater, err := NewTufAutoupdater(tufServerUrl, testRootDir, "", http.DefaultClient, tufServerUrl, http.DefaultClient, s, localservermocks.NewQuerier(t))
	require.NoError(t, err, "could not initialize new TUF autoupdater")

	// Update the metadata client with our test root JSON
	require.NoError(t, autoupdater.metadataClient.Init(rootJson), "could not initialize metadata client with test root JSON")

	// Set the check interval to something short so we can make a couple requests to our test metadata server
	autoupdater.checkInterval = 1 * time.Second

	// Set logger so that we can capture output
	var logBytes threadsafebuffer.ThreadSafeBuffer
	autoupdater.logger = log.NewJSONLogger(&logBytes)

	// Expect that we attempt to update the library
	mockLibrarian := newMockLibrarian(t)
	autoupdater.libraryManager = mockLibrarian
	mockLibrarian.On("AddToLibrary", binaryOsqueryd, fmt.Sprintf("osqueryd-%s.tar.gz", testReleaseVersion)).Return(nil)
	mockLibrarian.On("AddToLibrary", binaryLauncher, fmt.Sprintf("launcher-%s.tar.gz", testReleaseVersion)).Return(nil)

	// Let the autoupdater run for a bit
	go autoupdater.Execute()
	time.Sleep(5 * time.Second)

	// Shut down autoupdater
	autoupdater.Interrupt(errors.New("test error"))

	// Wait one second to let autoupdater shut down
	time.Sleep(1 * time.Second)

	// Assert expectation that we added the expected `testReleaseVersion` to the updates library
	mockLibrarian.AssertExpectations(t)

	// Check log lines to confirm that we see the log `received interrupt, stopping`, indicating that
	// the autoupdater shut down at the end
	logLines := strings.Split(strings.TrimSpace(logBytes.String()), "\n")

	// We expect at least 1 log for the shutdown line.
	require.GreaterOrEqual(t, len(logLines), 1)

	// Check that we shut down
	require.Contains(t, logLines[len(logLines)-1], "received interrupt, stopping")
}

func Test_storeError(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()
	testTufServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulates TUF server being down
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer testTufServer.Close()

	autoupdater, err := NewTufAutoupdater(testTufServer.URL, testRootDir, "", http.DefaultClient, testTufServer.URL, http.DefaultClient, setupStorage(t), localservermocks.NewQuerier(t))
	require.NoError(t, err, "could not initialize new TUF autoupdater")
	autoupdater.libraryManager = newMockLibrarian(t)

	// Set the check interval to something short so we can accumulate some errors
	autoupdater.checkInterval = 1 * time.Second

	// Start the autoupdater going
	go autoupdater.Execute()

	// Wait 5 seconds to accumulate errors, stop it, and give it a second to shut down
	time.Sleep(5 * time.Second)
	autoupdater.Interrupt(errors.New("test error"))
	time.Sleep(1 * time.Second)

	// Confirm that we saved the errors
	errorCount := 0
	err = autoupdater.store.ForEach(func(k, _ []byte) error {
		// Confirm error is saved with reasonable timestamp
		ts, err := strconv.ParseInt(string(k), 10, 64)
		require.NoError(t, err, "invalid timestamp in key: %s", string(k))
		require.LessOrEqual(t, time.Now().Unix()-ts, int64(30), "error saved under timestamp not within last 30 seconds")

		// Increment error count so we know we got something
		errorCount += 1
		return nil
	})
	require.NoError(t, err, "could not iterate over keys")
	require.Greater(t, errorCount, 0, "TUF autoupdater did not record error counts")
}

func Test_cleanUpOldErrors(t *testing.T) {
	t.Parallel()

	autoupdater := &TufAutoupdater{
		store:  setupStorage(t),
		logger: log.NewNopLogger(),
	}

	// Add one legitimate timestamp
	oneHourAgo := time.Now().Add(-1 * time.Hour).Unix()
	require.NoError(t, autoupdater.store.Set([]byte(fmt.Sprintf("%d", oneHourAgo)), []byte("{}")), "could not set recent timestamp for test")

	// Add some old timestamps
	eightDaysAgo := time.Now().Add(-8 * 24 * time.Hour).Unix()
	require.NoError(t, autoupdater.store.Set([]byte(fmt.Sprintf("%d", eightDaysAgo)), []byte("{}")), "could not set old timestamp for test")
	twoWeeksAgo := time.Now().Add(-14 * 24 * time.Hour).Unix()
	require.NoError(t, autoupdater.store.Set([]byte(fmt.Sprintf("%d", twoWeeksAgo)), []byte("{}")), "could not set old timestamp for test")

	// Add a malformed entry
	require.NoError(t, autoupdater.store.Set([]byte("not a timestamp"), []byte("{}")), "could not set old timestamp for test")

	// Confirm we added them
	keyCountBeforeCleanup := 0
	err := autoupdater.store.ForEach(func(_, _ []byte) error {
		keyCountBeforeCleanup += 1
		return nil
	})
	require.NoError(t, err, "could not iterate over keys")
	require.Equal(t, 4, keyCountBeforeCleanup, "did not correctly seed errors in bucket")

	// Call the cleanup function
	autoupdater.cleanUpOldErrors()

	keyCount := 0
	err = autoupdater.store.ForEach(func(_, _ []byte) error {
		keyCount += 1
		return nil
	})
	require.NoError(t, err, "could not iterate over keys")

	require.Equal(t, 1, keyCount, "cleanup routine did not clean up correct number of old errors")
}

// Sets up a local TUF repo with some targets to serve metadata about; returns the URL
// of a test HTTP server to serve that metadata and the root JSON needed to initialize
// a client.
func initLocalTufServer(t *testing.T, testReleaseVersion string) (tufServerURL string, rootJson []byte) {
	tufDir := t.TempDir()

	// Initialize repo with store
	localStore := tuf.FileSystemStore(tufDir, nil)
	repo, err := tuf.NewRepo(localStore)
	require.NoError(t, err, "could not create new tuf repo")
	require.NoError(t, repo.Init(false), "could not init new tuf repo")

	// Gen keys
	_, err = repo.GenKey("root")
	require.NoError(t, err, "could not gen root key")
	_, err = repo.GenKey("targets")
	require.NoError(t, err, "could not gen targets key")
	_, err = repo.GenKey("snapshot")
	require.NoError(t, err, "could not gen snapshot key")
	_, err = repo.GenKey("timestamp")
	require.NoError(t, err, "could not gen timestamp key")

	// Sign the root metadata file
	require.NoError(t, repo.Sign("root.json"), "could not sign root metadata file")

	// Create test binaries and release files per binary and per release channel
	for _, b := range binaries {
		for _, v := range []string{"0.1.1", "0.12.3-deadbeef", testReleaseVersion} {
			binaryFileName := fmt.Sprintf("%s-%s.tar.gz", b, v)

			// Create a valid test binary -- an archive of an executable with the proper directory structure
			// that will actually run -- if this is the release version we care about. If this is not the
			// release version we care about, then just create a small text file since it won't be downloaded
			// and evaluated.
			if v == testReleaseVersion {
				// Create test binary and copy it to the staged targets directory
				stagedTargetsDir := filepath.Join(tufDir, "staged", "targets", string(b), runtime.GOOS)
				executablePath := executableLocation(stagedTargetsDir, b)
				require.NoError(t, os.MkdirAll(filepath.Dir(executablePath), 0777), "could not make staging directory")
				copyBinary(t, executablePath)
				require.NoError(t, os.Chmod(executablePath, 0755))

				// Compress the binary or app bundle
				compress(t, binaryFileName, stagedTargetsDir, stagedTargetsDir, b)
			} else {
				// Create and commit a test binary
				require.NoError(t, os.MkdirAll(filepath.Join(tufDir, "staged", "targets", string(b), runtime.GOOS), 0777), "could not make staging directory")
				err = os.WriteFile(filepath.Join(tufDir, "staged", "targets", string(b), runtime.GOOS, binaryFileName), []byte("I am a test target"), 0777)
				require.NoError(t, err, "could not write test target binary to temp dir")
			}

			// Add the target
			require.NoError(t, repo.AddTarget(fmt.Sprintf("%s/%s/%s", b, runtime.GOOS, binaryFileName), nil), "could not add test target binary to tuf")

			// Commit
			require.NoError(t, repo.Snapshot(), "could not take snapshot")
			require.NoError(t, repo.Timestamp(), "could not take timestamp")
			require.NoError(t, repo.Commit(), "could not commit")

			if v != testReleaseVersion {
				continue
			}

			// If this is our release version, also create and commit a test release file
			for _, c := range []string{"stable", "beta", "nightly"} {
				require.NoError(t, os.MkdirAll(filepath.Join(tufDir, "staged", "targets", string(b), runtime.GOOS, c), 0777), "could not make staging directory")
				err = os.WriteFile(filepath.Join(tufDir, "staged", "targets", string(b), runtime.GOOS, c, "release.json"), []byte("{}"), 0777)
				require.NoError(t, err, "could not write test target release file to temp dir")
				customMetadata := fmt.Sprintf("{\"target\":\"%s/%s/%s\"}", b, runtime.GOOS, binaryFileName)
				require.NoError(t, repo.AddTarget(fmt.Sprintf("%s/%s/%s/release.json", b, runtime.GOOS, c), []byte(customMetadata)), "could not add test target release file to tuf")

				// Commit
				require.NoError(t, repo.Snapshot(), "could not take snapshot")
				require.NoError(t, repo.Timestamp(), "could not take timestamp")
				require.NoError(t, repo.Commit(), "could not commit")
			}
		}
	}

	// Quick validation that we set up the repo properly: key and metadata files should exist; targets should exist
	require.DirExists(t, filepath.Join(tufDir, "keys"))
	require.FileExists(t, filepath.Join(tufDir, "keys", "root.json"))
	require.FileExists(t, filepath.Join(tufDir, "keys", "snapshot.json"))
	require.FileExists(t, filepath.Join(tufDir, "keys", "targets.json"))
	require.FileExists(t, filepath.Join(tufDir, "keys", "timestamp.json"))
	require.DirExists(t, filepath.Join(tufDir, "repository"))
	require.FileExists(t, filepath.Join(tufDir, "repository", "root.json"))
	require.FileExists(t, filepath.Join(tufDir, "repository", "snapshot.json"))
	require.FileExists(t, filepath.Join(tufDir, "repository", "timestamp.json"))
	require.FileExists(t, filepath.Join(tufDir, "repository", "targets.json"))
	require.FileExists(t, filepath.Join(tufDir, "repository", "targets", "launcher", runtime.GOOS, "stable", "release.json"))
	require.FileExists(t, filepath.Join(tufDir, "repository", "targets", "launcher", runtime.GOOS, fmt.Sprintf("launcher-%s.tar.gz", testReleaseVersion)))
	require.FileExists(t, filepath.Join(tufDir, "repository", "targets", "osqueryd", runtime.GOOS, "stable", "release.json"))
	require.FileExists(t, filepath.Join(tufDir, "repository", "targets", "osqueryd", runtime.GOOS, fmt.Sprintf("osqueryd-%s.tar.gz", testReleaseVersion)))

	// Set up a test server to serve these files
	testMetadataServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pathComponents := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")

		fileToServe := tufDir

		// Allow the test server to also stand in for dl.kolide.co
		if pathComponents[0] == "kolide" {
			fileToServe = filepath.Join(fileToServe, "repository", "targets")
		} else {
			fileToServe = filepath.Join(fileToServe, pathComponents[0])
		}

		for i := 1; i < len(pathComponents); i += 1 {
			fileToServe = filepath.Join(fileToServe, pathComponents[i])
		}

		http.ServeFile(w, r, fileToServe)
	}))

	// Make sure we close the server at the end of our test
	t.Cleanup(func() {
		testMetadataServer.Close()
	})

	tufServerURL = testMetadataServer.URL

	metadata, err := repo.GetMeta()
	require.NoError(t, err, "could not get metadata from test TUF repo")
	require.Contains(t, metadata, "root.json")
	rootJson = metadata["root.json"]

	return tufServerURL, rootJson
}

func compress(t *testing.T, outFileName string, outFileDir string, targetDir string, binary autoupdatableBinary) {
	out, err := os.Create(filepath.Join(outFileDir, outFileName))
	require.NoError(t, err, "creating archive: %s in %s", outFileName, outFileDir)
	defer out.Close()

	gw := gzip.NewWriter(out)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	srcFilePath := string(binary)
	if binary == "launcher" && runtime.GOOS == "darwin" {
		srcFilePath = filepath.Join("Kolide.app", "Contents", "MacOS", string(binary))

		// Create directory structure for app bundle
		for _, path := range []string{"Kolide.app", "Kolide.app/Contents", "Kolide.app/Contents/MacOS"} {
			pInfo, err := os.Stat(filepath.Join(targetDir, path))
			require.NoError(t, err, "stat for app bundle path %s", path)

			hdr, err := tar.FileInfoHeader(pInfo, path)
			require.NoError(t, err, "creating header for directory %s", path)
			hdr.Name = path

			require.NoError(t, tw.WriteHeader(hdr), "writing tar header")
		}
	} else if runtime.GOOS == "windows" {
		srcFilePath += ".exe"
	}

	srcFile, err := os.Open(filepath.Join(targetDir, srcFilePath))
	require.NoError(t, err, "opening binary")
	defer srcFile.Close()

	srcStats, err := srcFile.Stat()
	require.NoError(t, err, "getting stats to compress binary")

	hdr, err := tar.FileInfoHeader(srcStats, srcStats.Name())
	require.NoError(t, err, "creating header")
	hdr.Name = srcFilePath

	require.NoError(t, tw.WriteHeader(hdr), "writing tar header")
	_, err = io.Copy(tw, srcFile)
	require.NoError(t, err, "copying file to archive")
}

func setupStorage(t *testing.T) types.KVStore {
	s, err := storageci.NewStore(t, log.NewNopLogger(), storage.AutoupdateErrorsStore.String())
	require.NoError(t, err)
	return s
}
