package tuf

import (
	"errors"
	"fmt"
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

	_, err := NewTufAutoupdater("https://example.com", testRootDir, http.DefaultClient, s)
	require.NoError(t, err, "could not initialize new TUF autoupdater")

	// Confirm that the TUF directory we expose is the one that we created
	exposedRootDir := LocalTufDirectory(testRootDir)

	_, err = os.Stat(exposedRootDir)
	require.NoError(t, err, "could not stat TUF directory that should have been initialized in test")

	_, err = os.Stat(filepath.Join(exposedRootDir, "root.json"))
	require.NoError(t, err, "could not stat root.json that should have been created in test")
}

// Tests running as well as shutdown
func TestExecute(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()
	testReleaseVersion := "1.2.3"
	metadataServerUrl, rootJson := initLocalTufServer(t, testReleaseVersion)
	s := setupStorage(t)

	// Right now, we do not talk to the mirror at all
	autoupdater, err := NewTufAutoupdater(metadataServerUrl, testRootDir, http.DefaultClient, s)
	require.NoError(t, err, "could not initialize new TUF autoupdater")

	// Update the metadata client with our test root JSON
	require.NoError(t, autoupdater.metadataClient.Init(rootJson), "could not initialize metadata client with test root JSON")

	// Set the check interval to something short so we can make a couple requests to our test metadata server
	autoupdater.checkInterval = 1 * time.Second

	// Set logger so that we can capture output
	var logBytes threadsafebuffer.ThreadSafeBuffer
	autoupdater.logger = log.NewJSONLogger(&logBytes)

	// Let the autoupdater run for a bit
	go autoupdater.Execute()
	time.Sleep(5 * time.Second)

	// Shut down autoupdater
	autoupdater.Interrupt(errors.New("test error"))

	// Wait one second to let autoupdater shut down
	time.Sleep(1 * time.Second)

	// Check log lines to confirm that:
	// 1. We were able to successfully pull updates from TUF and identified the expected release version
	// 2. We see the log `received interrupt, stopping`, indicating that the autoupdater shut down at the end
	logLines := strings.Split(strings.TrimSpace(logBytes.String()), "\n")

	// We expect 6 logs (1 check per second for 5 seconds, plus 1 log indicating shutdown) but will check
	// only the first and last, so just make sure there are at least 2
	require.GreaterOrEqual(t, len(logLines), 2)

	// Check that we got the expected release version
	require.Contains(t, logLines[0], testReleaseVersion)

	// Check that we shut down
	require.Contains(t, logLines[len(logLines)-1], "received interrupt, stopping")
}

func Test_storeError(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()
	testMetadataServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulates TUF server being down
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer testMetadataServer.Close()

	autoupdater, err := NewTufAutoupdater(testMetadataServer.URL, testRootDir, http.DefaultClient, setupStorage(t))
	require.NoError(t, err, "could not initialize new TUF autoupdater")

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

func Test_versionFromTarget(t *testing.T) {
	t.Parallel()

	testLauncherVersions := []struct {
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

	for _, testLauncherVersion := range testLauncherVersions {
		autoupdater := &TufAutoupdater{
			operatingSystem: testLauncherVersion.operatingSystem,
		}
		require.Equal(t, testLauncherVersion.version, autoupdater.versionFromTarget(testLauncherVersion.target, testLauncherVersion.binary))
	}
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

	// Gen keys
	_, err = repo.GenKey("root")
	require.NoError(t, err, "could not gen root key")
	_, err = repo.GenKey("targets")
	require.NoError(t, err, "could not gen root key")
	_, err = repo.GenKey("snapshot")
	require.NoError(t, err, "could not gen root key")
	_, err = repo.GenKey("timestamp")
	require.NoError(t, err, "could not gen root key")

	// Sign the root metadata file
	require.NoError(t, repo.Sign("root.json"), "could not sign root metadata file")

	// Create test binaries and release files per binary and per release channel
	for _, b := range []string{"launcher", "osqueryd"} {
		for _, c := range []string{"stable", "beta", "nightly"} {
			for _, v := range []string{"0.1.1", "0.12.3-deadbeef", testReleaseVersion} {
				// Create and commit a test binary
				binaryFileName := fmt.Sprintf("%s-%s.tar.gz", b, v)
				require.NoError(t, os.MkdirAll(filepath.Join(tufDir, "staged", "targets", b, runtime.GOOS), 0777), "could not make staging directory")
				err = os.WriteFile(filepath.Join(tufDir, "staged", "targets", b, runtime.GOOS, binaryFileName), []byte("I am a test target"), 0777)
				require.NoError(t, err, "could not write test target binary to temp dir")
				require.NoError(t, repo.AddTarget(fmt.Sprintf("%s/%s/%s", b, runtime.GOOS, binaryFileName), nil), "could not add test target binary to tuf")

				// Commit
				require.NoError(t, repo.Snapshot(), "could not take snapshot")
				require.NoError(t, repo.Timestamp(), "could not take timestamp")
				require.NoError(t, repo.Commit(), "could not commit")

				if v != testReleaseVersion {
					continue
				}

				// If this is our release version, also create and commit a test release file
				require.NoError(t, os.MkdirAll(filepath.Join(tufDir, "staged", "targets", b, runtime.GOOS, c), 0777), "could not make staging directory")
				err = os.WriteFile(filepath.Join(tufDir, "staged", "targets", b, runtime.GOOS, c, "release.json"), []byte("{}"), 0777)
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

	// Quick validation that we set up the repo properly: key and metadata files should exist
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

	// Set up a test server to serve these files
	testMetadataServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pathComponents := strings.Split(r.URL.Path, "/")
		fileToServe := tufDir
		for _, c := range pathComponents {
			fileToServe = filepath.Join(fileToServe, c)
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

func setupStorage(t *testing.T) types.KVStore {
	s, err := storageci.NewStore(t, log.NewNopLogger(), AutoupdateErrorBucket)
	require.NoError(t, err)
	return s
}
