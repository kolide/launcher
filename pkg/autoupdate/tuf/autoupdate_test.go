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

	"github.com/Masterminds/semver"
	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/agent/storage"
	storageci "github.com/kolide/launcher/pkg/agent/storage/ci"
	"github.com/kolide/launcher/pkg/agent/types"
	typesmocks "github.com/kolide/launcher/pkg/agent/types/mocks"
	tufci "github.com/kolide/launcher/pkg/autoupdate/tuf/ci"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNewTufAutoupdater(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()
	s := setupStorage(t)
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("RootDirectory").Return(testRootDir)
	mockKnapsack.On("UpdateChannel").Return("nightly")
	mockKnapsack.On("AutoupdateInterval").Return(60 * time.Second)
	mockKnapsack.On("AutoupdateErrorsStore").Return(s)
	mockKnapsack.On("TufServerURL").Return("https://example.com")
	mockKnapsack.On("UpdateDirectory").Return("")
	mockKnapsack.On("MirrorServerURL").Return("https://example.com")

	_, err := NewTufAutoupdater(mockKnapsack, http.DefaultClient, http.DefaultClient, newMockQuerier(t))
	require.NoError(t, err, "could not initialize new TUF autoupdater")

	// Confirm we pulled all config items as expected
	mockKnapsack.AssertExpectations(t)

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
	tufServerUrl, rootJson := tufci.InitRemoteTufServer(t, testReleaseVersion)
	s := setupStorage(t)
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("RootDirectory").Return(testRootDir)
	mockKnapsack.On("UpdateChannel").Return("nightly")
	mockKnapsack.On("AutoupdateInterval").Return(60 * time.Second)
	mockKnapsack.On("AutoupdateErrorsStore").Return(s)
	mockKnapsack.On("TufServerURL").Return(tufServerUrl)
	mockKnapsack.On("UpdateDirectory").Return("")
	mockKnapsack.On("MirrorServerURL").Return("https://example.com")
	mockQuerier := newMockQuerier(t)

	// Set up autoupdater
	autoupdater, err := NewTufAutoupdater(mockKnapsack, http.DefaultClient, http.DefaultClient, mockQuerier)
	require.NoError(t, err, "could not initialize new TUF autoupdater")

	// Confirm we pulled all config items as expected
	mockKnapsack.AssertExpectations(t)

	// Update the metadata client with our test root JSON
	require.NoError(t, autoupdater.metadataClient.Init(rootJson), "could not initialize metadata client with test root JSON")

	// Set the check interval to something short so we can make a couple requests to our test metadata server
	autoupdater.checkInterval = 1 * time.Second

	// Set logger so that we can capture output
	var logBytes threadsafebuffer.ThreadSafeBuffer
	autoupdater.logger = log.NewJSONLogger(&logBytes)

	// Get metadata for each release
	_, err = autoupdater.metadataClient.Update()
	require.NoError(t, err, "could not update metadata client to fetch target metadata")
	osquerydMetadata, err := autoupdater.metadataClient.Target(fmt.Sprintf("%s/%s/%s-%s.tar.gz", binaryOsqueryd, runtime.GOOS, binaryOsqueryd, testReleaseVersion))
	require.NoError(t, err, "could not get test metadata for osqueryd")
	launcherMetadata, err := autoupdater.metadataClient.Target(fmt.Sprintf("%s/%s/%s-%s.tar.gz", binaryLauncher, runtime.GOOS, binaryLauncher, testReleaseVersion))
	require.NoError(t, err, "could not get test metadata for launcher")

	// Expect that we attempt to tidy the library first before running execute loop
	mockLibraryManager := NewMocklibrarian(t)
	autoupdater.libraryManager = mockLibraryManager
	mockQuerier.On("Query", mock.Anything).Return([]map[string]string{{"version": "1.1.1"}}, nil).Once()
	mockLibraryManager.On("TidyLibrary", binaryOsqueryd, mock.Anything).Return().Once()

	// Expect that we attempt to update the library
	mockLibraryManager.On("Available", binaryOsqueryd, fmt.Sprintf("osqueryd-%s.tar.gz", testReleaseVersion)).Return(false)
	mockLibraryManager.On("Available", binaryLauncher, fmt.Sprintf("launcher-%s.tar.gz", testReleaseVersion)).Return(false)
	mockLibraryManager.On("AddToLibrary", binaryOsqueryd, fmt.Sprintf("osqueryd-%s.tar.gz", testReleaseVersion), osquerydMetadata).Return(nil)
	mockLibraryManager.On("AddToLibrary", binaryLauncher, fmt.Sprintf("launcher-%s.tar.gz", testReleaseVersion), launcherMetadata).Return(nil)

	// Let the autoupdater run for a bit
	go autoupdater.Execute()
	time.Sleep(5 * time.Second)

	// Shut down autoupdater
	autoupdater.Interrupt(errors.New("test error"))

	// Wait one second to let autoupdater shut down
	time.Sleep(1 * time.Second)

	// Assert expectation that we added the expected `testReleaseVersion` to the updates library
	mockLibraryManager.AssertExpectations(t)

	// Check log lines to confirm that we see the log `received interrupt, stopping`, indicating that
	// the autoupdater shut down at the end
	logLines := strings.Split(strings.TrimSpace(logBytes.String()), "\n")

	// We expect at least 1 log for the shutdown line.
	require.GreaterOrEqual(t, len(logLines), 1)

	// Check that we shut down
	require.Contains(t, logLines[len(logLines)-1], "received interrupt, stopping")
}

func Test_currentRunningVersion_launcher_errorWhenVersionIsNotSet(t *testing.T) {
	t.Parallel()

	mockQuerier := newMockQuerier(t)
	autoupdater := &TufAutoupdater{
		logger:    log.NewNopLogger(),
		osquerier: mockQuerier,
	}

	// In test, version.Version() returns `unknown` for everything, which is not something
	// that the semver library can parse. So we only expect an error here.
	launcherVersion, err := autoupdater.currentRunningVersion("launcher")
	require.Error(t, err, "expected an error fetching current running version of launcher")
	require.Equal(t, "", launcherVersion)
}

func Test_currentRunningVersion_osqueryd(t *testing.T) {
	t.Parallel()

	mockQuerier := newMockQuerier(t)
	autoupdater := &TufAutoupdater{
		logger:    log.NewNopLogger(),
		osquerier: mockQuerier,
	}

	// Expect to return one row containing the version
	expectedOsqueryVersion, err := semver.NewVersion("5.10.12")
	require.NoError(t, err)
	mockQuerier.On("Query", mock.Anything).Return([]map[string]string{{"version": expectedOsqueryVersion.Original()}}, nil).Once()

	osqueryVersion, err := autoupdater.currentRunningVersion("osqueryd")
	require.NoError(t, err, "expected no error fetching current running version of osqueryd")
	require.Equal(t, expectedOsqueryVersion.Original(), osqueryVersion)
}

func Test_currentRunningVersion_osqueryd_handlesQueryError(t *testing.T) {
	t.Parallel()

	mockQuerier := newMockQuerier(t)
	autoupdater := &TufAutoupdater{
		logger:                 log.NewNopLogger(),
		osquerier:              mockQuerier,
		osquerierRetryInterval: 1 * time.Millisecond,
	}

	// Expect to return an error (five times, since we perform retries)
	mockQuerier.On("Query", mock.Anything).Return(make([]map[string]string, 0), errors.New("test osqueryd querying error"))

	osqueryVersion, err := autoupdater.currentRunningVersion("osqueryd")
	require.Error(t, err, "expected an error returning osquery version when querying osquery fails")
	require.Equal(t, "", osqueryVersion)
}

func Test_storeError(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()
	testTufServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulates TUF server being down
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer testTufServer.Close()
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("RootDirectory").Return(testRootDir)
	mockKnapsack.On("UpdateChannel").Return("nightly")
	mockKnapsack.On("AutoupdateInterval").Return(60 * time.Second)
	mockKnapsack.On("AutoupdateErrorsStore").Return(setupStorage(t))
	mockKnapsack.On("TufServerURL").Return(testTufServer.URL)
	mockKnapsack.On("UpdateDirectory").Return("")
	mockKnapsack.On("MirrorServerURL").Return("https://example.com")
	mockQuerier := newMockQuerier(t)

	autoupdater, err := NewTufAutoupdater(mockKnapsack, http.DefaultClient, http.DefaultClient, mockQuerier)
	require.NoError(t, err, "could not initialize new TUF autoupdater")

	// Confirm we pulled all config items as expected
	mockKnapsack.AssertExpectations(t)

	mockLibraryManager := NewMocklibrarian(t)
	autoupdater.libraryManager = mockLibraryManager
	mockQuerier.On("Query", mock.Anything).Return([]map[string]string{{"version": "1.1.1"}}, nil).Once()

	// We only expect TidyLibrary to run for osqueryd, since we can't get the current running version
	// for launcher in tests.
	mockLibraryManager.On("TidyLibrary", binaryOsqueryd, mock.Anything).Return().Once()

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

	mockLibraryManager.AssertExpectations(t)
	mockQuerier.AssertExpectations(t)
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

func setupStorage(t *testing.T) types.KVStore {
	s, err := storageci.NewStore(t, log.NewNopLogger(), storage.AutoupdateErrorsStore.String())
	require.NoError(t, err)
	return s
}
