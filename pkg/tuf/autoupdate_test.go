package tuf

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
	"github.com/theupdateframework/go-tuf"
)

func TestNewTufAutoupdater(t *testing.T) {
	t.Parallel()

	binaryPath := "some/path/to/launcher"
	testRootDir := t.TempDir()

	_, err := NewTufAutoupdater("https://example.com", binaryPath, testRootDir)
	require.NoError(t, err, "could not initialize new TUF autoupdater")

	_, err = os.Stat(filepath.Join(testRootDir, "launcher-tuf-dev"))
	require.NoError(t, err, "could not stat TUF directory that should have been initialized in test")

	_, err = os.Stat(filepath.Join(testRootDir, "launcher-tuf-dev", "root.json"))
	require.NoError(t, err, "could not stat root.json that should have been created in test")
}

// Tests running as well as shutdown
func TestRun(t *testing.T) {
	t.Parallel()

	for _, binary := range []string{"launcher", "osqueryd"} {
		binary := binary
		t.Run(fmt.Sprintf("TestRun: %s", binary), func(t *testing.T) {
			t.Parallel()
			binaryPath := filepath.Join("some", "path", "to", binary)
			testRootDir := t.TempDir()
			testReleaseVersion := "1.2.3"
			metadataServerUrl, rootJson := initLocalTufServer(t, testReleaseVersion)

			// Right now, we do not talk to the mirror at all
			autoupdater, err := NewTufAutoupdater(metadataServerUrl, binaryPath, testRootDir)
			require.NoError(t, err, "could not initialize new TUF autoupdater")

			// Update the metadata client with our test root JSON
			require.NoError(t, autoupdater.metadataClient.Init(rootJson), "could not initialize metadata client with test root JSON")

			// Set the check interval to something short so we can make a couple requests to our test metadata server
			autoupdater.checkInterval = 1 * time.Second

			// Set logger so that we can capture output
			var logBytes threadsafebuffer.ThreadSafeBuffer
			autoupdater.logger = log.NewJSONLogger(&logBytes)

			// Let the autoupdater run for a bit
			stop, err := autoupdater.Run()
			require.NoError(t, err, "could not run TUF autoupdater")
			time.Sleep(5 * time.Second)

			// Shut down autoupdater
			stop()

			// Wait one second to let autoupdater shut down
			time.Sleep(1 * time.Second)

			// Check log lines to confirm that:
			// 1. We were able to successfully pull updates from TUF and identified the expected release version
			// 2. We see the log `received interrupt, stopping`, indicating that the autoupdater shut down at the end
			logLines := strings.Split(strings.TrimSpace(logBytes.String()), "\n")

			// TODO RM - just for debugging tests
			for _, l := range logLines {
				fmt.Println(l)
			}

			// We expect 6 logs (1 check per second for 5 seconds, plus 1 log indicating shutdown) but will check
			// only the first and last, so just make sure there are at least 2
			require.GreaterOrEqual(t, len(logLines), 2)

			// Check that we got the expected release version
			require.Contains(t, logLines[0], testReleaseVersion)

			// Check that we shut down
			require.Contains(t, logLines[len(logLines)-1], "received interrupt, stopping")
		})
	}
}

func TestRollingErrorCount(t *testing.T) {
	t.Parallel()

	binaryPath := "some/path/to/launcher"
	testRootDir := t.TempDir()
	testMetadataServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulates TUF server being down
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer testMetadataServer.Close()

	autoupdater, err := NewTufAutoupdater(testMetadataServer.URL, binaryPath, testRootDir)
	require.NoError(t, err, "could not initialize new TUF autoupdater")

	// Set the check interval to something short so we can accumulate some errors
	autoupdater.checkInterval = 1 * time.Second

	// Start the autoupdater going
	stop, err := autoupdater.Run()
	require.NoError(t, err, "could not run TUF autoupdater")

	// Wait 5 seconds to accumulate errors, stop it, and give it a second to shut down
	time.Sleep(5 * time.Second)
	stop()
	time.Sleep(1 * time.Second)

	// Confirm that we saved the errors with correct timestamps
	require.Greater(t, autoupdater.RollingErrorCount(), 0, "TUF autoupdater did not record error counts")
}

func TestRollingErrorCount_Respects24HourWindow(t *testing.T) {
	t.Parallel()

	autoupdater := &TufAutoupdater{
		errorCounter: make([]int64, 0),
	}

	// Add one legitimate timestamp
	autoupdater.errorCounter = append(autoupdater.errorCounter, time.Now().Add(-1*time.Hour).Unix())

	// Add some old timestamps
	autoupdater.errorCounter = append(autoupdater.errorCounter, time.Now().Add(-28*time.Hour).Unix())
	autoupdater.errorCounter = append(autoupdater.errorCounter, time.Now().Add(-48*time.Hour).Unix())
	autoupdater.errorCounter = append(autoupdater.errorCounter, time.Now().Add(-49*time.Hour).Unix())
	autoupdater.errorCounter = append(autoupdater.errorCounter, time.Now().Add(-50*time.Hour).Unix())

	require.Equal(t, 1, autoupdater.RollingErrorCount(), "TUF autoupdater did not correctly determine which timestamps to include for error count")
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
			binary:          "launcher.exe",
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
			binary:          "osqueryd.exe",
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
			binary:          testLauncherVersion.binary,
			operatingSystem: testLauncherVersion.operatingSystem,
		}
		require.Equal(t, testLauncherVersion.version, autoupdater.versionFromTarget(testLauncherVersion.target))
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
				require.NoError(t, repo.AddTarget(filepath.Join(b, runtime.GOOS, binaryFileName), nil), "could not add test target binary to tuf")

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
				require.NoError(t, repo.AddTarget(filepath.Join(b, runtime.GOOS, c, "release.json"), []byte(customMetadata)), "could not add test target release file to tuf")

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
