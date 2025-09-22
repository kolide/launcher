package tuf

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Masterminds/semver"
	"github.com/kolide/launcher/ee/agent/flags/keys"
	typesmocks "github.com/kolide/launcher/ee/agent/types/mocks"
	tufci "github.com/kolide/launcher/ee/tuf/ci"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/kolide/launcher/pkg/threadsafebuffer"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theupdateframework/go-tuf/data"
)

func TestNewTufAutoupdater(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("RootDirectory").Return(testRootDir)
	mockKnapsack.On("TufServerURL").Return("https://example.com")
	mockKnapsack.On("UpdateDirectory").Return("")
	mockKnapsack.On("MirrorServerURL").Return("https://example.com")
	mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())
	mockKnapsack.On("UpdateChannel").Return("nightly")
	mockKnapsack.On("AutoupdateInitialDelay").Return(0 * time.Second)
	mockKnapsack.On("PinnedLauncherVersion").Return("")
	mockKnapsack.On("PinnedOsquerydVersion").Return("")
	mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel, keys.PinnedLauncherVersion, keys.PinnedOsquerydVersion, keys.AutoupdateDownloadSplay).Return()

	_, err := NewTufAutoupdater(context.TODO(), mockKnapsack, http.DefaultClient, http.DefaultClient)
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

func TestExecute_launcherUpdate(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()
	testReleaseVersion := "1.2.3"
	tufServerUrl, rootJson := tufci.InitRemoteTufServer(t, testReleaseVersion)

	// Set up osquery binary
	osqBinaryPath := filepath.Join(t.TempDir(), "osqueryd")
	if runtime.GOOS == "windows" {
		osqBinaryPath += ".exe"
	}
	tufci.CopyOlderBinary(t, osqBinaryPath)

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("RootDirectory").Return(testRootDir)
	mockKnapsack.On("UpdateChannel").Return("nightly")
	mockKnapsack.On("PinnedLauncherVersion").Return("")
	mockKnapsack.On("PinnedOsquerydVersion").Return("")
	mockKnapsack.On("AutoupdateInterval").Return(500 * time.Millisecond)
	mockKnapsack.On("AutoupdateInitialDelay").Return(0 * time.Second)
	mockKnapsack.On("TufServerURL").Return(tufServerUrl)
	mockKnapsack.On("UpdateDirectory").Return("")
	mockKnapsack.On("MirrorServerURL").Return("https://example.com")
	mockKnapsack.On("LocalDevelopmentPath").Return("")
	mockKnapsack.On("InModernStandby").Return(false)
	mockKnapsack.On("LatestOsquerydPath", mock.Anything).Return(osqBinaryPath)
	mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel, keys.PinnedLauncherVersion, keys.PinnedOsquerydVersion, keys.AutoupdateDownloadSplay).Return()
	mockKnapsack.On("AutoupdateDownloadSplay").Return(0 * time.Second)

	// Set logger so that we can capture output
	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := multislogger.New(slog.NewJSONHandler(&logBytes, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mockKnapsack.On("Slogger").Return(slogger.Logger)

	// Set up autoupdater
	autoupdater, err := NewTufAutoupdater(context.TODO(), mockKnapsack, http.DefaultClient, http.DefaultClient)
	require.NoError(t, err, "could not initialize new TUF autoupdater")

	// Update the metadata client with our test root JSON
	require.NoError(t, autoupdater.metadataClient.Init(rootJson), "could not initialize metadata client with test root JSON")

	// Get metadata for each release
	_, err = autoupdater.metadataClient.Update()
	require.NoError(t, err, "could not update metadata client to fetch target metadata")
	osquerydMetadata, err := autoupdater.metadataClient.Target(fmt.Sprintf("%s/%s/%s/%s-%s.tar.gz", binaryOsqueryd, runtime.GOOS, PlatformArch(), binaryOsqueryd, testReleaseVersion))
	require.NoError(t, err, "could not get test metadata for osqueryd")
	launcherMetadata, err := autoupdater.metadataClient.Target(fmt.Sprintf("%s/%s/%s/%s-%s.tar.gz", binaryLauncher, runtime.GOOS, PlatformArch(), binaryLauncher, testReleaseVersion))
	require.NoError(t, err, "could not get test metadata for launcher")

	// Expect that we attempt to tidy the library first before running execute loop
	mockLibraryManager := NewMocklibrarian(t)
	autoupdater.libraryManager = mockLibraryManager
	currentLauncherVersion := "" // cannot determine using version package in test
	mockLibraryManager.On("TidyLibrary", binaryOsqueryd, mock.Anything).Return().Once()

	// Expect that we attempt to update the library
	mockLibraryManager.On("Available", binaryOsqueryd, fmt.Sprintf("osqueryd-%s.tar.gz", testReleaseVersion)).Return(false).Once()
	mockLibraryManager.On("Available", binaryOsqueryd, fmt.Sprintf("osqueryd-%s.tar.gz", testReleaseVersion)).Return(true).Maybe() // On subsequent iterations, no need to download again
	mockLibraryManager.On("Available", binaryLauncher, fmt.Sprintf("launcher-%s.tar.gz", testReleaseVersion)).Return(false).Once()
	mockLibraryManager.On("Available", binaryLauncher, fmt.Sprintf("launcher-%s.tar.gz", testReleaseVersion)).Return(true).Maybe() // On subsequent iterations, no need to download again
	mockLibraryManager.On("AddToLibrary", binaryOsqueryd, tufci.OlderBinaryVersion, fmt.Sprintf("osqueryd-%s.tar.gz", testReleaseVersion), osquerydMetadata).Return(nil).Once()
	mockLibraryManager.On("AddToLibrary", binaryLauncher, currentLauncherVersion, fmt.Sprintf("launcher-%s.tar.gz", testReleaseVersion), launcherMetadata).Return(nil).Once()

	// Let the autoupdater run for a bit -- it will shut itself down after a launcher update
	go autoupdater.Execute()

	// Wait up to 5 seconds to confirm we see log lines `received interrupt to restart launcher after update, stopping`, indicating that
	// the autoupdater shut down at the end
	shutdownDeadline := time.Now().Add(5 * time.Second).Unix()
	for {
		if time.Now().Unix() > shutdownDeadline {
			t.Error("autoupdater did not shut down within 5 seconds -- logs: ", logBytes.String())
			t.FailNow()
		}

		// Wait for Execute to do its thing
		time.Sleep(100 * time.Millisecond)

		logLines := strings.Split(strings.TrimSpace(logBytes.String()), "\n")
		autoupdaterInterrupted := false
		for _, line := range logLines {
			if strings.Contains(line, "received interrupt to restart launcher after update, stopping") {
				autoupdaterInterrupted = true
				break
			}
		}

		if autoupdaterInterrupted {
			break
		}
	}

	// Assert expectation that we added the expected `testReleaseVersion` to the updates library
	mockLibraryManager.AssertExpectations(t)

	// Confirm we pulled all config items as expected
	mockKnapsack.AssertExpectations(t)
}

func TestExecute_osquerydUpdate(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()
	testReleaseVersion := "1.2.3"
	tufServerUrl, rootJson := tufci.InitRemoteTufServer(t, testReleaseVersion)

	// Set up osquery binary
	osqBinaryPath := filepath.Join(t.TempDir(), "osqueryd")
	if runtime.GOOS == "windows" {
		osqBinaryPath += ".exe"
	}
	tufci.CopyOlderBinary(t, osqBinaryPath)

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("RootDirectory").Return(testRootDir)
	mockKnapsack.On("UpdateChannel").Return("nightly")
	mockKnapsack.On("PinnedLauncherVersion").Return("")
	mockKnapsack.On("PinnedOsquerydVersion").Return("")
	mockKnapsack.On("AutoupdateInterval").Return(100 * time.Millisecond) // Set the check interval to something short so we can make a couple requests to our test metadata server
	mockKnapsack.On("AutoupdateInitialDelay").Return(0 * time.Second)
	mockKnapsack.On("TufServerURL").Return(tufServerUrl)
	mockKnapsack.On("UpdateDirectory").Return("")
	mockKnapsack.On("MirrorServerURL").Return("https://example.com")
	mockKnapsack.On("InModernStandby").Return(false)
	mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel, keys.PinnedLauncherVersion, keys.PinnedOsquerydVersion, keys.AutoupdateDownloadSplay).Return()
	mockKnapsack.On("LatestOsquerydPath", mock.Anything).Return(osqBinaryPath)
	mockKnapsack.On("AutoupdateDownloadSplay").Return(0 * time.Second)

	// Set logger so that we can capture output
	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := multislogger.New(slog.NewJSONHandler(&logBytes, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mockKnapsack.On("Slogger").Return(slogger.Logger)

	// Set up autoupdater
	autoupdater, err := NewTufAutoupdater(context.TODO(), mockKnapsack, http.DefaultClient, http.DefaultClient, WithOsqueryRestart(func(context.Context) error { return nil }))
	require.NoError(t, err, "could not initialize new TUF autoupdater")
	autoupdater.osqueryTimeout = 5 * time.Second

	// Update the metadata client with our test root JSON
	require.NoError(t, autoupdater.metadataClient.Init(rootJson), "could not initialize metadata client with test root JSON")

	// Get metadata for each release
	_, err = autoupdater.metadataClient.Update()
	require.NoError(t, err, "could not update metadata client to fetch target metadata")
	osquerydMetadata, err := autoupdater.metadataClient.Target(fmt.Sprintf("%s/%s/%s/%s-%s.tar.gz", binaryOsqueryd, runtime.GOOS, PlatformArch(), binaryOsqueryd, testReleaseVersion))
	require.NoError(t, err, "could not get test metadata for osqueryd")

	// Expect that we attempt to tidy the library first before running execute loop
	mockLibraryManager := NewMocklibrarian(t)
	autoupdater.libraryManager = mockLibraryManager
	mockLibraryManager.On("TidyLibrary", binaryOsqueryd, mock.Anything).Return().Once()

	// Expect that we attempt to update the library
	mockLibraryManager.On("Available", binaryOsqueryd, fmt.Sprintf("osqueryd-%s.tar.gz", testReleaseVersion)).Return(false)
	mockLibraryManager.On("Available", binaryLauncher, fmt.Sprintf("launcher-%s.tar.gz", testReleaseVersion)).Return(true)
	mockLibraryManager.On("AddToLibrary", binaryOsqueryd, tufci.OlderBinaryVersion, fmt.Sprintf("osqueryd-%s.tar.gz", testReleaseVersion), osquerydMetadata).Return(nil)

	// Let the autoupdater run for a bit
	go autoupdater.Execute()
	time.Sleep(2 * autoupdater.osqueryTimeout)

	// Assert expectation that we added the expected `testReleaseVersion` to the updates library
	mockLibraryManager.AssertExpectations(t)

	// Check log lines to confirm that we see the log `restarted binary after update`, indicating that
	// we restarted osqueryd
	logLines := strings.Split(strings.TrimSpace(logBytes.String()), "\n")

	// We expect at least 1 log for the restart line.
	require.GreaterOrEqual(t, len(logLines), 1)

	// Check that we restarted osqueryd
	restartFound := false
	for _, logLine := range logLines {
		if strings.Contains(logLine, "restarted binary after update") {
			restartFound = true
			break
		}
	}
	require.True(t, restartFound, fmt.Sprintf("logs missing restart: %s", strings.Join(logLines, "\n")))

	// The autoupdater won't stop after an osqueryd download, so interrupt it and let it shut down
	autoupdater.Interrupt(errors.New("test error"))
	time.Sleep(100 * time.Millisecond)

	// Confirm we pulled all config items as expected
	mockKnapsack.AssertExpectations(t)
}

func TestExecute_downgrade(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()
	testReleaseVersion := "3.22.9"
	tufServerUrl, rootJson := tufci.InitRemoteTufServer(t, testReleaseVersion)

	// Set up osquery binary
	osqBinaryPath := filepath.Join(t.TempDir(), "osqueryd")
	if runtime.GOOS == "windows" {
		osqBinaryPath += ".exe"
	}
	tufci.CopyBinary(t, osqBinaryPath)

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("RootDirectory").Return(testRootDir)
	mockKnapsack.On("UpdateChannel").Return("nightly")
	mockKnapsack.On("PinnedLauncherVersion").Return("")
	mockKnapsack.On("PinnedOsquerydVersion").Return("")
	mockKnapsack.On("AutoupdateInterval").Return(100 * time.Millisecond) // Set the check interval to something short so we can make a couple requests to our test metadata server
	mockKnapsack.On("AutoupdateInitialDelay").Return(0 * time.Second)
	mockKnapsack.On("TufServerURL").Return(tufServerUrl)
	mockKnapsack.On("UpdateDirectory").Return("")
	mockKnapsack.On("MirrorServerURL").Return("https://example.com")
	mockKnapsack.On("InModernStandby").Return(false)
	mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel, keys.PinnedLauncherVersion, keys.PinnedOsquerydVersion, keys.AutoupdateDownloadSplay).Return()
	mockKnapsack.On("LatestOsquerydPath", mock.Anything).Return(osqBinaryPath)

	// Set logger so that we can capture output
	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := multislogger.New(slog.NewJSONHandler(&logBytes, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mockKnapsack.On("Slogger").Return(slogger.Logger)

	// Set up autoupdater
	autoupdater, err := NewTufAutoupdater(context.TODO(), mockKnapsack, http.DefaultClient, http.DefaultClient, WithOsqueryRestart(func(context.Context) error { return nil }))
	require.NoError(t, err, "could not initialize new TUF autoupdater")

	// Update the metadata client with our test root JSON
	require.NoError(t, autoupdater.metadataClient.Init(rootJson), "could not initialize metadata client with test root JSON")

	// Get metadata for each release
	_, err = autoupdater.metadataClient.Update()
	require.NoError(t, err, "could not update metadata client to fetch target metadata")

	// Expect that we attempt to tidy the library first before running execute loop
	mockLibraryManager := NewMocklibrarian(t)
	autoupdater.libraryManager = mockLibraryManager
	mockLibraryManager.On("TidyLibrary", binaryOsqueryd, mock.Anything).Return().Once()

	// Expect that we do not attempt to update the library (i.e. the osquery update was previously downloaded)
	mockLibraryManager.On("Available", binaryOsqueryd, fmt.Sprintf("osqueryd-%s.tar.gz", testReleaseVersion)).Return(true)
	mockLibraryManager.On("Available", binaryLauncher, fmt.Sprintf("launcher-%s.tar.gz", testReleaseVersion)).Return(true)

	// Let the autoupdater run for a bit
	go autoupdater.Execute()

	// Wait up to 5 seconds to confirm we see log lines `restarted binary after update`, indicating that
	// the autoupdater restarted osqueryd even though it did not perform a download
	shutdownDeadline := time.Now().Add(5 * time.Second).Unix()
	for {
		if time.Now().Unix() > shutdownDeadline {
			t.Error("autoupdater did not restart osquery within 5 seconds -- logs: ", logBytes.String())
			t.FailNow()
		}

		// Wait for Execute to do its thing
		time.Sleep(100 * time.Millisecond)

		logLines := strings.Split(strings.TrimSpace(logBytes.String()), "\n")
		osquerydRestarted := false
		for _, line := range logLines {
			if strings.Contains(line, "restarted binary after update") {
				osquerydRestarted = true
				break
			}
		}

		if osquerydRestarted {
			break
		}
	}

	// Assert expectation that we checked to see if version was available in library
	mockLibraryManager.AssertExpectations(t)

	// The autoupdater won't stop after an osqueryd download, so interrupt it and let it shut down
	autoupdater.Interrupt(errors.New("test error"))
	time.Sleep(100 * time.Millisecond)

	// Confirm we pulled all config items as expected
	mockKnapsack.AssertExpectations(t)
}

func TestExecute_withInitialDelay(t *testing.T) {
	t.Parallel()

	initialDelay := 500 * time.Millisecond

	testRootDir := t.TempDir()
	testReleaseVersion := "1.2.3"
	tufServerUrl, _ := tufci.InitRemoteTufServer(t, testReleaseVersion)
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("RootDirectory").Return(testRootDir)
	mockKnapsack.On("AutoupdateInitialDelay").Return(initialDelay)
	mockKnapsack.On("TufServerURL").Return(tufServerUrl)
	mockKnapsack.On("UpdateDirectory").Return("")
	mockKnapsack.On("MirrorServerURL").Return("https://example.com")
	mockKnapsack.On("UpdateChannel").Return("nightly")
	mockKnapsack.On("PinnedLauncherVersion").Return("")
	mockKnapsack.On("PinnedOsquerydVersion").Return("")
	mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel, keys.PinnedLauncherVersion, keys.PinnedOsquerydVersion, keys.AutoupdateDownloadSplay).Return()

	// Set logger so that we can capture output
	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := multislogger.New(slog.NewJSONHandler(&logBytes, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mockKnapsack.On("Slogger").Return(slogger.Logger)

	// Set up autoupdater
	autoupdater, err := NewTufAutoupdater(context.TODO(), mockKnapsack, http.DefaultClient, http.DefaultClient,
		WithOsqueryRestart(func(context.Context) error { return nil }))
	require.NoError(t, err, "could not initialize new TUF autoupdater")

	// Expect that we interrupt
	mockLibraryManager := NewMocklibrarian(t)
	autoupdater.libraryManager = mockLibraryManager

	// Let the autoupdater run for less than the initial delay
	go autoupdater.Execute()
	time.Sleep(300 * time.Millisecond)

	// Shut down the autoupdater
	autoupdater.Interrupt(errors.New("test error"))
	time.Sleep(100 * time.Millisecond)

	// Assert expectation that we closed the library
	mockLibraryManager.AssertExpectations(t)

	// Check log lines to confirm that we see the log `received external interrupt during initial delay, stopping`,
	// indicating that we halted during the initial delay
	logLines := strings.Split(strings.TrimSpace(logBytes.String()), "\n")

	// We expect at least 1 log for the shutdown line.
	require.GreaterOrEqual(t, len(logLines), 1)

	// Check that we shut down
	require.Contains(t, logLines[len(logLines)-1], "received external interrupt during initial delay, stopping")

	// Confirm we pulled all config items as expected
	mockKnapsack.AssertExpectations(t)
}

func TestExecute_inModernStandby(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()
	testReleaseVersion := "1.2.3"
	tufServerUrl, _ := tufci.InitRemoteTufServer(t, testReleaseVersion)

	// Set up osquery binary
	osqBinaryPath := filepath.Join(t.TempDir(), "osqueryd")
	if runtime.GOOS == "windows" {
		osqBinaryPath += ".exe"
	}
	tufci.CopyBinary(t, osqBinaryPath)

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("RootDirectory").Return(testRootDir)
	mockKnapsack.On("AutoupdateInterval").Return(100 * time.Millisecond) // Set the check interval to something short so we can make a couple requests to our test metadata server
	mockKnapsack.On("AutoupdateInitialDelay").Return(0 * time.Second)
	mockKnapsack.On("TufServerURL").Return(tufServerUrl)
	mockKnapsack.On("UpdateDirectory").Return("")
	mockKnapsack.On("MirrorServerURL").Return("https://example.com")
	mockKnapsack.On("UpdateChannel").Return("nightly")
	mockKnapsack.On("PinnedLauncherVersion").Return("")
	mockKnapsack.On("PinnedOsquerydVersion").Return("")
	mockKnapsack.On("InModernStandby").Return(true)
	mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())
	mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel, keys.PinnedLauncherVersion, keys.PinnedOsquerydVersion, keys.AutoupdateDownloadSplay).Return()
	mockKnapsack.On("LatestOsquerydPath", mock.Anything).Return(osqBinaryPath)

	// Set up autoupdater
	autoupdater, err := NewTufAutoupdater(context.TODO(), mockKnapsack, http.DefaultClient, http.DefaultClient,
		WithOsqueryRestart(func(context.Context) error { return nil }))
	require.NoError(t, err, "could not initialize new TUF autoupdater")
	autoupdater.osqueryTimeout = 5 * time.Second

	// Set up library manager: we should expect to tidy the library on startup, but NOT add anything to it
	mockLibraryManager := NewMocklibrarian(t)
	autoupdater.libraryManager = mockLibraryManager
	mockLibraryManager.On("TidyLibrary", binaryOsqueryd, mock.Anything).Return().Once()

	// Let the autoupdater run for long enough to make it through tidying the library (which includes an osquery version query)
	// and at least starting an autoupdate check
	go autoupdater.Execute()
	time.Sleep(2 * autoupdater.osqueryTimeout)

	// Shut down the autoupdater
	autoupdater.Interrupt(errors.New("test error"))
	time.Sleep(100 * time.Millisecond)

	// Confirm we didn't attempt any library updates, and pulled config items as expected
	mockLibraryManager.AssertExpectations(t)
	mockKnapsack.AssertExpectations(t)
}

func TestInterrupt_Multiple(t *testing.T) {
	t.Parallel()

	// We don't need a real metadata server for this one
	testMetadataServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("{}"))
	}))

	// Make sure we close the server at the end of our test
	t.Cleanup(func() {
		testMetadataServer.Close()
	})

	testRootDir := t.TempDir()

	// Set up osquery binary
	osqBinaryPath := filepath.Join(t.TempDir(), "osqueryd")
	if runtime.GOOS == "windows" {
		osqBinaryPath += ".exe"
	}
	tufci.CopyBinary(t, osqBinaryPath)

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("RootDirectory").Return(testRootDir)
	mockKnapsack.On("AutoupdateInterval").Return(60 * time.Second)
	mockKnapsack.On("AutoupdateInitialDelay").Return(0 * time.Second)
	mockKnapsack.On("TufServerURL").Return(testMetadataServer.URL)
	mockKnapsack.On("UpdateDirectory").Return("")
	mockKnapsack.On("MirrorServerURL").Return("https://example.com")
	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	mockKnapsack.On("Slogger").Return(slogger)
	mockKnapsack.On("UpdateChannel").Return("nightly")
	mockKnapsack.On("PinnedLauncherVersion").Return("")
	mockKnapsack.On("PinnedOsquerydVersion").Return("")
	mockKnapsack.On("InModernStandby").Return(false)
	mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel, keys.PinnedLauncherVersion, keys.PinnedOsquerydVersion, keys.AutoupdateDownloadSplay).Return()
	mockKnapsack.On("LatestOsquerydPath", mock.Anything).Return(osqBinaryPath)

	// Set up autoupdater
	autoupdater, err := NewTufAutoupdater(context.TODO(), mockKnapsack, http.DefaultClient, http.DefaultClient,
		WithOsqueryRestart(func(context.Context) error { return nil }))
	require.NoError(t, err, "could not initialize new TUF autoupdater")
	autoupdater.osqueryTimeout = 5 * time.Second

	// Set up normal library and querier interactions
	mockLibraryManager := NewMocklibrarian(t)
	autoupdater.libraryManager = mockLibraryManager
	mockLibraryManager.On("TidyLibrary", binaryOsqueryd, mock.Anything).Return().Once()

	// Let the autoupdater run for long enough to make it through tidying the library (which includes an osquery version query)
	// and at least starting an autoupdate check
	go autoupdater.Execute()
	time.Sleep(2 * autoupdater.osqueryTimeout)

	interruptStart := time.Now()
	autoupdater.Interrupt(errors.New("test error"))

	// Confirm we can call Interrupt multiple times without blocking
	interruptComplete := make(chan struct{})
	expectedInterrupts := 3
	for i := 0; i < expectedInterrupts; i += 1 {
		go func() {
			autoupdater.Interrupt(nil)
			interruptComplete <- struct{}{}
		}()
	}

	receivedInterrupts := 0
	for {
		if receivedInterrupts >= expectedInterrupts {
			break
		}

		select {
		case <-interruptComplete:
			receivedInterrupts += 1
			continue
		case <-time.After(5 * time.Second):
			t.Errorf("could not call interrupt multiple times and return within 5 seconds -- interrupted at %s, received %d interrupts before timeout; logs: \n%s\n", interruptStart.String(), receivedInterrupts, logBytes.String())
			t.FailNow()
		}
	}

	require.Equal(t, expectedInterrupts, receivedInterrupts)

	// Confirm we pulled all config items as expected
	mockKnapsack.AssertExpectations(t)
}

func TestDo(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		updateData controlServerAutoupdateRequest
	}{
		{
			name: "just launcher",
			updateData: controlServerAutoupdateRequest{
				BinariesToUpdate: []binaryToUpdate{
					{
						Name: "launcher",
					},
				},
			},
		},
		{
			name: "just osqueryd",
			updateData: controlServerAutoupdateRequest{
				BinariesToUpdate: []binaryToUpdate{
					{
						Name: "osqueryd",
					},
				},
			},
		},
		{
			name: "both binaries",
			updateData: controlServerAutoupdateRequest{
				BinariesToUpdate: []binaryToUpdate{
					{
						Name: "launcher",
					},
					{
						Name: "osqueryd",
					},
				},
			},
		},
		{
			name: "both binaries plus an invalid binary",
			updateData: controlServerAutoupdateRequest{
				BinariesToUpdate: []binaryToUpdate{
					{
						Name: "launcher",
					},
					{
						Name: "osqueryd",
					},
					{
						Name: "some_unknown_binary",
					},
				},
			},
		},
	}
	for _, tt := range testCases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testRootDir := t.TempDir()
			testReleaseVersion := "2.2.3"
			tufServerUrl, rootJson := tufci.InitRemoteTufServer(t, testReleaseVersion)

			// Set up osquery binary
			osqBinaryPath := filepath.Join(t.TempDir(), "osqueryd")
			if runtime.GOOS == "windows" {
				osqBinaryPath += ".exe"
			}
			tufci.CopyOlderBinary(t, osqBinaryPath)

			mockKnapsack := typesmocks.NewKnapsack(t)
			mockKnapsack.On("RootDirectory").Return(testRootDir)
			mockKnapsack.On("UpdateChannel").Return("nightly")
			mockKnapsack.On("PinnedLauncherVersion").Return("")
			mockKnapsack.On("PinnedOsquerydVersion").Return("")
			mockKnapsack.On("AutoupdateInitialDelay").Return(0 * time.Second)
			mockKnapsack.On("TufServerURL").Return(tufServerUrl)
			mockKnapsack.On("UpdateDirectory").Return("")
			mockKnapsack.On("MirrorServerURL").Return("https://example.com")
			mockKnapsack.On("LocalDevelopmentPath").Return("").Maybe()
			mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())
			mockKnapsack.On("InModernStandby").Return(false)
			mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel, keys.PinnedLauncherVersion, keys.PinnedOsquerydVersion, keys.AutoupdateDownloadSplay).Return()
			mockKnapsack.On("LatestOsquerydPath", mock.Anything).Return(osqBinaryPath).Maybe()
			// Set up autoupdater
			autoupdater, err := NewTufAutoupdater(context.TODO(), mockKnapsack, http.DefaultClient, http.DefaultClient, WithOsqueryRestart(func(context.Context) error { return nil }))
			require.NoError(t, err, "could not initialize new TUF autoupdater")

			// Update the metadata client with our test root JSON
			require.NoError(t, autoupdater.metadataClient.Init(rootJson), "could not initialize metadata client with test root JSON")

			// Get metadata for each release
			_, err = autoupdater.metadataClient.Update()
			require.NoError(t, err, "could not update metadata client to fetch target metadata")

			// Expect that we attempt to update the library, only for the selected, valid binary/binaries
			mockLibraryManager := NewMocklibrarian(t)
			autoupdater.libraryManager = mockLibraryManager
			for _, b := range tt.updateData.BinariesToUpdate {
				if b.Name != "osqueryd" && b.Name != "launcher" {
					continue
				}

				mockLibraryManager.On("Available", autoupdatableBinary(b.Name), fmt.Sprintf("%s-%s.tar.gz", b.Name, testReleaseVersion)).Return(false)
				mockLibraryManager.On("AddToLibrary", autoupdatableBinary(b.Name), mock.Anything, mock.Anything, mock.Anything).Return(nil)
			}

			// Prepare control server request
			rawRequest, err := json.Marshal(tt.updateData)
			require.NoError(t, err, "marshalling update request")
			data := bytes.NewReader(rawRequest)

			// Make request
			require.NoError(t, autoupdater.Do(data), "expected no error making update request")

			// Assert expectation that we added the expected `testReleaseVersion` to the updates library
			mockLibraryManager.AssertExpectations(t)

			// Confirm we pulled all config items as expected
			mockKnapsack.AssertExpectations(t)
		})
	}
}

func TestDo_HandlesSimultaneousUpdates(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()
	testReleaseVersion := "1.5.0"
	tufServerUrl, rootJson := tufci.InitRemoteTufServer(t, testReleaseVersion)

	// Set up osquery binary
	osqBinaryPath := filepath.Join(t.TempDir(), "osqueryd")
	if runtime.GOOS == "windows" {
		osqBinaryPath += ".exe"
	}
	tufci.CopyOlderBinary(t, osqBinaryPath)

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("RootDirectory").Return(testRootDir)
	mockKnapsack.On("UpdateChannel").Return("nightly")
	mockKnapsack.On("PinnedLauncherVersion").Return("")
	mockKnapsack.On("PinnedOsquerydVersion").Return("")
	interval := 500 * time.Millisecond
	mockKnapsack.On("AutoupdateInterval").Return(interval)
	mockKnapsack.On("AutoupdateInitialDelay").Return(0 * time.Millisecond)
	mockKnapsack.On("TufServerURL").Return(tufServerUrl)
	mockKnapsack.On("UpdateDirectory").Return("")
	mockKnapsack.On("MirrorServerURL").Return("https://example.com")
	mockKnapsack.On("LocalDevelopmentPath").Return("")
	mockKnapsack.On("InModernStandby").Return(false)
	mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())
	mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel, keys.PinnedLauncherVersion, keys.PinnedOsquerydVersion, keys.AutoupdateDownloadSplay).Return()
	mockKnapsack.On("LatestOsquerydPath", mock.Anything).Return(osqBinaryPath)
	mockKnapsack.On("AutoupdateDownloadSplay").Return(0 * time.Second)

	// Set up autoupdater
	autoupdater, err := NewTufAutoupdater(context.TODO(), mockKnapsack, http.DefaultClient, http.DefaultClient, WithOsqueryRestart(func(context.Context) error { return nil }))
	require.NoError(t, err, "could not initialize new TUF autoupdater")

	// Update the metadata client with our test root JSON
	require.NoError(t, autoupdater.metadataClient.Init(rootJson), "could not initialize metadata client with test root JSON")

	// Get metadata for each release
	_, err = autoupdater.metadataClient.Update()
	require.NoError(t, err, "could not update metadata client to fetch target metadata")

	// Expect that we attempt to tidy the library first before running execute loop
	mockLibraryManager := NewMocklibrarian(t)
	autoupdater.libraryManager = mockLibraryManager
	mockLibraryManager.On("TidyLibrary", binaryOsqueryd, mock.Anything).Return().Once()

	// Expect that we attempt to update the library, only for the selected binary/binaries
	autoupdater.libraryManager = mockLibraryManager
	for _, b := range binaries {
		mockLibraryManager.On("Available", b, fmt.Sprintf("%s-%s.tar.gz", string(b), testReleaseVersion)).Return(false)
		mockLibraryManager.On("AddToLibrary", b, mock.Anything, mock.Anything, mock.Anything).Return(nil) // TODO once?
	}

	// Prepare control server request
	rawRequest, err := json.Marshal(controlServerAutoupdateRequest{
		BinariesToUpdate: []binaryToUpdate{
			{
				Name: "launcher",
			},
			{
				Name: "osqueryd",
			},
		},
	})
	require.NoError(t, err, "marshalling update request")
	data := bytes.NewReader(rawRequest)

	// Start the autoupdater, then make the control server request
	go autoupdater.Execute()
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, autoupdater.Do(data), "expected no error making update request")

	// Give autoupdater a chance to run
	time.Sleep(interval)

	// Assert expectation that we added the expected `testReleaseVersion` to the updates library
	mockLibraryManager.AssertExpectations(t)

	// Confirm we pulled all config items as expected
	mockKnapsack.AssertExpectations(t)
}

func TestDo_WillNotExecuteDuringInitialDelay(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()
	testReleaseVersion := "1.5.0"
	tufServerUrl, rootJson := tufci.InitRemoteTufServer(t, testReleaseVersion)

	// Set up osquery binary
	osqBinaryPath := filepath.Join(t.TempDir(), "osqueryd")
	if runtime.GOOS == "windows" {
		osqBinaryPath += ".exe"
	}
	tufci.CopyOlderBinary(t, osqBinaryPath)

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("RootDirectory").Return(testRootDir)
	mockKnapsack.On("UpdateChannel").Return("nightly")
	mockKnapsack.On("PinnedLauncherVersion").Return("")
	mockKnapsack.On("PinnedOsquerydVersion").Return("")
	interval := 100 * time.Millisecond
	mockKnapsack.On("AutoupdateInterval").Return(interval)
	initialDelay := 1 * time.Second
	mockKnapsack.On("AutoupdateInitialDelay").Return(initialDelay)
	mockKnapsack.On("TufServerURL").Return(tufServerUrl)
	mockKnapsack.On("UpdateDirectory").Return("")
	mockKnapsack.On("MirrorServerURL").Return("https://example.com")
	mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())
	mockKnapsack.On("InModernStandby").Return(false)
	mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel, keys.PinnedLauncherVersion, keys.PinnedOsquerydVersion, keys.AutoupdateDownloadSplay).Return()
	mockKnapsack.On("LatestOsquerydPath", mock.Anything).Return(osqBinaryPath)

	// Set up autoupdater
	autoupdater, err := NewTufAutoupdater(context.TODO(), mockKnapsack, http.DefaultClient, http.DefaultClient, WithOsqueryRestart(func(context.Context) error { return nil }))
	require.NoError(t, err, "could not initialize new TUF autoupdater")

	// Update the metadata client with our test root JSON
	require.NoError(t, autoupdater.metadataClient.Init(rootJson), "could not initialize metadata client with test root JSON")

	// Get metadata for each release
	_, err = autoupdater.metadataClient.Update()
	require.NoError(t, err, "could not update metadata client to fetch target metadata")

	// Set up expectations for Execute function: tidying library, checking for updates
	mockLibraryManager := NewMocklibrarian(t)
	autoupdater.libraryManager = mockLibraryManager
	mockLibraryManager.On("TidyLibrary", binaryOsqueryd, mock.Anything).Return().Once()
	autoupdater.libraryManager = mockLibraryManager
	for _, b := range binaries {
		mockLibraryManager.On("Available", b, fmt.Sprintf("%s-%s.tar.gz", string(b), testReleaseVersion)).Return(true)
	}

	// Prepare control server request
	rawRequest, err := json.Marshal(controlServerAutoupdateRequest{
		BinariesToUpdate: []binaryToUpdate{
			{
				Name: "launcher",
			},
			{
				Name: "osqueryd",
			},
		},
	})
	require.NoError(t, err, "marshalling update request")
	data := bytes.NewReader(rawRequest)

	// Start the autoupdater, then make the control server request right away, during the initial delay
	go autoupdater.Execute()
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, autoupdater.Do(data), "should not have received error when performing request during initial delay")

	// Give autoupdater a chance to run
	time.Sleep(2*initialDelay + 2*interval)

	// Assert expectation that we did not add the expected `testReleaseVersion` to the updates library
	mockLibraryManager.AssertExpectations(t)

	// Confirm we pulled all config items as expected
	mockKnapsack.AssertExpectations(t)
}

func TestFlagsChanged_UpdateChannelChanged(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()
	testReleaseVersion := "2.2.3"
	tufServerUrl, rootJson := tufci.InitRemoteTufServer(t, testReleaseVersion)

	// Set up osquery binary
	osqBinaryPath := filepath.Join(t.TempDir(), "osqueryd")
	if runtime.GOOS == "windows" {
		osqBinaryPath += ".exe"
	}
	tufci.CopyOlderBinary(t, osqBinaryPath)

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("RootDirectory").Return(testRootDir)
	mockKnapsack.On("TufServerURL").Return(tufServerUrl)
	mockKnapsack.On("UpdateDirectory").Return("")
	mockKnapsack.On("MirrorServerURL").Return("https://example.com")
	mockKnapsack.On("AutoupdateInitialDelay").Return(0 * time.Second)
	mockKnapsack.On("LocalDevelopmentPath").Return("").Maybe()
	mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())
	mockKnapsack.On("PinnedLauncherVersion").Return("")
	mockKnapsack.On("PinnedOsquerydVersion").Return("")
	mockKnapsack.On("InModernStandby").Return(false)
	mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel, keys.PinnedLauncherVersion, keys.PinnedOsquerydVersion, keys.AutoupdateDownloadSplay).Return()
	mockKnapsack.On("LatestOsquerydPath", mock.Anything).Return(osqBinaryPath)

	// Start out on beta channel, then swap to nightly
	mockKnapsack.On("UpdateChannel").Return("beta").Once()
	mockKnapsack.On("UpdateChannel").Return("nightly")

	// Set up autoupdater
	autoupdater, err := NewTufAutoupdater(context.TODO(), mockKnapsack, http.DefaultClient, http.DefaultClient, WithOsqueryRestart(func(context.Context) error { return nil }))
	require.NoError(t, err, "could not initialize new TUF autoupdater")
	require.Equal(t, "beta", autoupdater.updateChannel)

	// Update the metadata client with our test root JSON
	require.NoError(t, autoupdater.metadataClient.Init(rootJson), "could not initialize metadata client with test root JSON")

	// Get metadata for each release
	_, err = autoupdater.metadataClient.Update()
	require.NoError(t, err, "could not update metadata client to fetch target metadata")

	// Expect that we attempt to update the library
	mockLibraryManager := NewMocklibrarian(t)
	autoupdater.libraryManager = mockLibraryManager
	mockLibraryManager.On("Available", binaryOsqueryd, fmt.Sprintf("%s-%s.tar.gz", binaryOsqueryd, testReleaseVersion)).Return(false)
	mockLibraryManager.On("AddToLibrary", binaryOsqueryd, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mockLibraryManager.On("Available", binaryLauncher, fmt.Sprintf("%s-%s.tar.gz", binaryLauncher, testReleaseVersion)).Return(false)
	mockLibraryManager.On("AddToLibrary", binaryLauncher, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	// Notify that flags changed
	autoupdater.FlagsChanged(context.TODO(), keys.UpdateChannel)

	// Assert expectation that we added the expected `testReleaseVersion` to the updates library
	mockLibraryManager.AssertExpectations(t)

	// Confirm we pulled all config items as expected
	mockKnapsack.AssertExpectations(t)

	// Confirm we're on the expected update channel
	require.Equal(t, "nightly", autoupdater.updateChannel)
}

func TestFlagsChanged_PinnedVersionChanged(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()
	pinnedOsquerydVersion := "5.11.0"
	tufServerUrl, rootJson := tufci.InitRemoteTufServer(t, pinnedOsquerydVersion)

	// Set up osquery binary
	osqBinaryPath := filepath.Join(t.TempDir(), "osqueryd")
	if runtime.GOOS == "windows" {
		osqBinaryPath += ".exe"
	}
	tufci.CopyOlderBinary(t, osqBinaryPath)

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("RootDirectory").Return(testRootDir)
	mockKnapsack.On("TufServerURL").Return(tufServerUrl)
	mockKnapsack.On("UpdateDirectory").Return("")
	mockKnapsack.On("MirrorServerURL").Return("https://example.com")
	mockKnapsack.On("LocalDevelopmentPath").Return("").Maybe()
	mockKnapsack.On("AutoupdateInitialDelay").Return(0 * time.Second)
	mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())
	mockKnapsack.On("UpdateChannel").Return("nightly")
	mockKnapsack.On("PinnedLauncherVersion").Return("")
	mockKnapsack.On("InModernStandby").Return(false)
	mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel, keys.PinnedLauncherVersion, keys.PinnedOsquerydVersion, keys.AutoupdateDownloadSplay).Return()
	mockKnapsack.On("LatestOsquerydPath", mock.Anything).Return(osqBinaryPath)

	// Start out with no pinned version, then set a pinned version
	mockKnapsack.On("PinnedOsquerydVersion").Return("").Once()
	mockKnapsack.On("PinnedOsquerydVersion").Return(pinnedOsquerydVersion)

	// Set up autoupdater
	autoupdater, err := NewTufAutoupdater(context.TODO(), mockKnapsack, http.DefaultClient, http.DefaultClient, WithOsqueryRestart(func(context.Context) error { return nil }))
	require.NoError(t, err, "could not initialize new TUF autoupdater")
	require.Equal(t, "", autoupdater.pinnedVersions[binaryOsqueryd])

	// Update the metadata client with our test root JSON
	require.NoError(t, autoupdater.metadataClient.Init(rootJson), "could not initialize metadata client with test root JSON")

	// Get metadata for each release
	_, err = autoupdater.metadataClient.Update()
	require.NoError(t, err, "could not update metadata client to fetch target metadata")

	// Expect that we attempt to update the library
	mockLibraryManager := NewMocklibrarian(t)
	autoupdater.libraryManager = mockLibraryManager
	mockLibraryManager.On("Available", binaryOsqueryd, fmt.Sprintf("%s-%s.tar.gz", binaryOsqueryd, pinnedOsquerydVersion)).Return(false)
	mockLibraryManager.On("AddToLibrary", binaryOsqueryd, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	// Notify that flags changed
	autoupdater.FlagsChanged(context.TODO(), keys.PinnedOsquerydVersion)

	// Assert expectation that we added the expected `testReleaseVersion` to the updates library
	mockLibraryManager.AssertExpectations(t)

	// Confirm we pulled all config items as expected
	mockKnapsack.AssertExpectations(t)

	// Confirm we have the current osqueryd version set
	require.Equal(t, pinnedOsquerydVersion, autoupdater.pinnedVersions[binaryOsqueryd])
}

func TestFlagsChanged_DuringInitialDelay(t *testing.T) {
	t.Parallel()

	testRootDir := t.TempDir()
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("RootDirectory").Return(testRootDir)
	mockKnapsack.On("UpdateChannel").Return("nightly")
	mockKnapsack.On("PinnedOsquerydVersion").Return("")
	interval := 100 * time.Millisecond
	mockKnapsack.On("AutoupdateInterval").Return(interval).Maybe()
	initialDelay := 1 * time.Second
	mockKnapsack.On("AutoupdateInitialDelay").Return(initialDelay)
	mockKnapsack.On("TufServerURL").Return("https://example.com")
	mockKnapsack.On("UpdateDirectory").Return("")
	mockKnapsack.On("MirrorServerURL").Return("https://example.com")
	mockKnapsack.On("Slogger").Return(multislogger.NewNopLogger())
	mockKnapsack.On("RegisterChangeObserver", mock.Anything, keys.UpdateChannel, keys.PinnedLauncherVersion, keys.PinnedOsquerydVersion, keys.AutoupdateDownloadSplay).Return()

	// Start out with a pinned version, then unset the pinned version
	pinnedLauncherVersion := "1.7.3"
	mockKnapsack.On("PinnedLauncherVersion").Return(pinnedLauncherVersion).Once()
	mockKnapsack.On("PinnedLauncherVersion").Return("")

	// Set up autoupdater
	autoupdater, err := NewTufAutoupdater(context.TODO(), mockKnapsack, http.DefaultClient, http.DefaultClient, WithOsqueryRestart(func(context.Context) error { return nil }))
	require.NoError(t, err, "could not initialize new TUF autoupdater")
	require.Equal(t, pinnedLauncherVersion, autoupdater.pinnedVersions[binaryLauncher])

	// Start the autoupdater, then notify flag change right away, during the initial delay
	go autoupdater.Execute()
	time.Sleep(100 * time.Millisecond)
	autoupdater.FlagsChanged(context.TODO(), keys.PinnedLauncherVersion)

	// Stop the autoupdater
	autoupdater.Interrupt(errors.New("test error"))

	// Confirm we unset the pinned launcher version
	require.Equal(t, "", autoupdater.pinnedVersions[binaryLauncher])

	// Confirm we pulled all config items as expected
	mockKnapsack.AssertExpectations(t)
}

func Test_currentRunningVersion_launcher_errorWhenVersionIsNotSet(t *testing.T) {
	t.Parallel()

	autoupdater := &TufAutoupdater{
		slogger: multislogger.NewNopLogger(),
	}

	// In test, version.Version() returns `unknown` for everything, which is not something
	// that the semver library can parse. So we only expect an error here.
	launcherVersion, err := autoupdater.currentRunningVersion("launcher")
	require.Error(t, err, "expected an error fetching current running version of launcher")
	require.Equal(t, "", launcherVersion)
}

func Test_currentRunningVersion_osqueryd(t *testing.T) {
	t.Parallel()

	mockKnapsack := typesmocks.NewKnapsack(t)
	testBinDir := t.TempDir()

	// Set up osquery binary
	osqBinaryPath := filepath.Join(testBinDir, "osqueryd")
	if runtime.GOOS == "windows" {
		osqBinaryPath += ".exe"
	}
	tufci.CopyOlderBinary(t, osqBinaryPath)

	mockKnapsack.On("LatestOsquerydPath", mock.Anything).Return(osqBinaryPath)

	autoupdater := &TufAutoupdater{
		slogger:        multislogger.NewNopLogger(),
		osqueryTimeout: 5 * time.Second,
		knapsack:       mockKnapsack,
	}

	// Expect to return one row containing the version
	expectedOsqueryVersion, err := semver.NewVersion(tufci.OlderBinaryVersion)
	require.NoError(t, err)

	osqueryVersion, err := autoupdater.currentRunningVersion("osqueryd")
	require.NoError(t, err, "expected no error fetching current running version of osqueryd")
	require.Equal(t, expectedOsqueryVersion.Original(), osqueryVersion)
}

func Test_currentRunningVersion_osqueryd_missing_binary(t *testing.T) {
	t.Parallel()

	mockKnapsack := typesmocks.NewKnapsack(t)
	testBinDir := t.TempDir()
	// create a tmp dir to point at but do not populate with osqueryd binary-
	// we expect to error immediately for the case of a missing osqueryd
	fakeOsqBinaryPath := executableLocation(testBinDir, "osqueryd")
	mockKnapsack.On("LatestOsquerydPath", mock.Anything).Return(fakeOsqBinaryPath)

	autoupdater := &TufAutoupdater{
		slogger:  multislogger.NewNopLogger(),
		knapsack: mockKnapsack,
	}

	osqueryVersion, err := autoupdater.currentRunningVersion("osqueryd")
	require.Error(t, err, "expected currentRunningVersion to err immediately for missing osqueryd binary")
	require.Equal(t, "", osqueryVersion, "expected no current osquery version to be returned")
}

//go:embed testdata/test_promote_time_target_files.json
var sampleTargetJson []byte

func getSampleTargets(t *testing.T) data.TargetFiles {
	var targetFiles data.TargetFiles
	err := json.Unmarshal(sampleTargetJson, &targetFiles)
	require.NoError(t, err, "expected to be able to unmarshal sample json into data.TargetFiles")
	return targetFiles
}

func Test_findReleasePromoteTime(t *testing.T) {
	t.Parallel()

	// note that the sample targets file json is pre-curated so that the the promotion times
	// match across OS and arch to simplify testing across platforms here
	targets := getSampleTargets(t)
	tests := []struct {
		name                string
		binary              autoupdatableBinary
		channel             string
		expectedPromoteTime int64
	}{
		{
			name:                "osqueryd with valid alpha targets",
			binary:              "osqueryd",
			channel:             "alpha",
			expectedPromoteTime: 1750955751,
		},
		{
			name:                "osqueryd with valid nightly targets",
			binary:              "osqueryd",
			channel:             "nightly",
			expectedPromoteTime: 1750870406,
		},
		{
			name:                "osqueryd with valid stable targets",
			binary:              "osqueryd",
			channel:             "stable",
			expectedPromoteTime: 1750954246,
		},
		{
			// note that this is testing our zero value behavior,
			// all beta targets are in the sample targets file are intentionally missing promote_time
			name:                "osqueryd with missing beta target promotion times",
			binary:              "osqueryd",
			channel:             "beta",
			expectedPromoteTime: 0,
		},
		{
			name:                "launcher with valid alpha targets",
			binary:              "launcher",
			channel:             "alpha",
			expectedPromoteTime: 1750961031,
		},
		{
			name:                "launcher with valid nightly targets",
			binary:              "launcher",
			channel:             "nightly",
			expectedPromoteTime: 1750859203,
		},
		{
			name:                "launcher with valid stable targets",
			binary:              "launcher",
			channel:             "stable",
			expectedPromoteTime: 1750955736,
		},
		{
			// note that this is testing our zero value behavior,
			// all beta targets are in the sample targets file are intentionally missing promote_time
			name:                "launcher with missing beta target promotion times",
			binary:              "launcher",
			channel:             "beta",
			expectedPromoteTime: 0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			promoteTime := findReleasePromoteTime(context.Background(), tt.binary, targets, tt.channel)
			require.Equal(t, tt.expectedPromoteTime, promoteTime)
		})
	}
}

func Test_shouldDelayDownloadRespectsDisabledSplay(t *testing.T) {
	t.Parallel()

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("AutoupdateDownloadSplay").Return(0 * time.Second)

	autoupdater := &TufAutoupdater{
		slogger:              multislogger.NewNopLogger(),
		knapsack:             mockKnapsack,
		calculatedSplayDelay: &atomic.Int64{},
	}

	require.False(t, autoupdater.shouldDelayDownload(autoupdatableBinary("osqueryd"), data.TargetFiles{}))
}

func Test_shouldDelayDownloadDoesNotDelayWithoutPromoteTime(t *testing.T) {
	t.Parallel()

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("AutoupdateDownloadSplay").Return(8 * time.Hour)

	autoupdater := &TufAutoupdater{
		slogger:              multislogger.NewNopLogger(),
		knapsack:             mockKnapsack,
		updateChannel:        "beta", // none of the beta targets have promote_time set
		calculatedSplayDelay: &atomic.Int64{},
	}

	targets := getSampleTargets(t)
	require.False(t, autoupdater.shouldDelayDownload(autoupdatableBinary("osqueryd"), targets))
	require.False(t, autoupdater.shouldDelayDownload(autoupdatableBinary("launcher"), targets))
}

func Test_shouldDelayDownloadDoesNotDelayIfPromoteTimeExceedsSplay(t *testing.T) {
	t.Parallel()

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("AutoupdateDownloadSplay").Return(2 * time.Hour)

	autoupdater := &TufAutoupdater{
		slogger:              multislogger.NewNopLogger(),
		knapsack:             mockKnapsack,
		updateChannel:        "alpha", // all promote_times in here will always be greater than 2 hours ago (splay time)
		calculatedSplayDelay: &atomic.Int64{},
	}

	targets := getSampleTargets(t)
	require.False(t, autoupdater.shouldDelayDownload(autoupdatableBinary("osqueryd"), targets))
	require.False(t, autoupdater.shouldDelayDownload(autoupdatableBinary("launcher"), targets))
}

func Test_shouldDelayDownloadDelaysIfPromotedWithinSplay(t *testing.T) {
	t.Parallel()

	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("AutoupdateDownloadSplay").Return(8 * time.Hour)

	autoupdater := &TufAutoupdater{
		slogger:              multislogger.NewNopLogger(),
		knapsack:             mockKnapsack,
		updateChannel:        "alpha",
		calculatedSplayDelay: &atomic.Int64{},
	}

	targets := getSampleTargets(t)
	for _, binary := range []string{"osqueryd", "launcher"} {
		// find the release file from samples and update the promote time to be within the splay time
		targetReleaseFile := path.Join(binary, runtime.GOOS, PlatformArch(), autoupdater.updateChannel, "release.json")
		existingTarget := targets[targetReleaseFile]
		// setting promote time to be 5 minutes after splay time so there's no way it should not be delayed, regardless of uuid hash or seed
		updatedPromote := fmt.Sprintf(`{"promote_time": %d}`, time.Now().Add(5*time.Minute).Unix())
		newCustomMetadata := json.RawMessage([]byte(updatedPromote))
		existingTarget.Custom = &newCustomMetadata
		targets[targetReleaseFile] = existingTarget

		require.True(t, autoupdater.shouldDelayDownload(autoupdatableBinary(binary), targets))
	}
}

func Test_splayDelaySecondsReturnsConsistentDelay(t *testing.T) {
	t.Parallel()

	downloadSplayDuration := 8 * time.Hour
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("AutoupdateDownloadSplay").Return(downloadSplayDuration)

	autoupdater := &TufAutoupdater{
		slogger:              multislogger.NewNopLogger(),
		knapsack:             mockKnapsack,
		updateChannel:        "alpha",
		calculatedSplayDelay: &atomic.Int64{},
	}
	initialDelay := autoupdater.getSplayDelaySeconds()

	// initial delay should have a minimum value of 1,
	// and should never exceed our AutoupdateDownloadSplay time in seconds
	require.GreaterOrEqual(t, initialDelay, int64(1))
	require.LessOrEqual(t, initialDelay, int64(downloadSplayDuration.Seconds()))

	for range 3 {
		require.Equal(t, initialDelay, autoupdater.getSplayDelaySeconds())
	}
}

func Test_FlagsChangedAutoupdateDownloadSplayResetsCalculatedSplay(t *testing.T) {
	t.Parallel()

	downloadSplayDuration := 8 * time.Hour
	mockKnapsack := typesmocks.NewKnapsack(t)
	mockKnapsack.On("AutoupdateDownloadSplay").Return(downloadSplayDuration)
	mockKnapsack.On("UpdateChannel").Return("alpha")

	autoupdater := &TufAutoupdater{
		slogger:              multislogger.NewNopLogger(),
		knapsack:             mockKnapsack,
		updateChannel:        "alpha",
		calculatedSplayDelay: &atomic.Int64{},
		pinnedVersions:       map[autoupdatableBinary]string{},
	}
	initialDelay := autoupdater.getSplayDelaySeconds()

	// initial delay should have a minimum value of 1,
	// and should never exceed our AutoupdateDownloadSplay time in seconds
	require.GreaterOrEqual(t, initialDelay, int64(1))
	require.LessOrEqual(t, initialDelay, int64(downloadSplayDuration.Seconds()))

	autoupdater.FlagsChanged(context.TODO(), keys.AutoupdateDownloadSplay)
	// verify that it was reset
	require.Equal(t, int64(0), autoupdater.calculatedSplayDelay.Load())

	// verify that it is repopulated on next read
	newDelay := autoupdater.getSplayDelaySeconds()
	require.GreaterOrEqual(t, newDelay, int64(1))
	require.LessOrEqual(t, newDelay, int64(downloadSplayDuration.Seconds()))
}
