package tuf

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kit/kit/log"
	tufci "github.com/kolide/launcher/pkg/autoupdate/tuf/ci"
	"github.com/stretchr/testify/require"
)

func TestNewUpdateLibraryLookup(t *testing.T) {
	t.Parallel()

	// Set up an update library
	rootDir := t.TempDir()
	updateDir := DefaultLibraryDirectory(rootDir)
	require.NoError(t, os.MkdirAll(filepath.Join(updateDir, "launcher"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(updateDir, "osqueryd"), 0755))

	// Set up a local TUF repo
	require.NoError(t, os.MkdirAll(LocalTufDirectory(rootDir), 0755))

	// Create new library lookup
	_, err := NewUpdateLibraryLookup(rootDir, "", "stable", log.NewNopLogger())
	require.NoError(t, err)
}

func TestNewUpdateLibraryLookup_cannotInitMetadataClient(t *testing.T) {
	t.Parallel()

	// Set up an update library, but no TUF repo
	rootDir := t.TempDir()
	updateDir := DefaultLibraryDirectory(rootDir)
	require.NoError(t, os.MkdirAll(filepath.Join(updateDir, "launcher"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(updateDir, "osqueryd"), 0755))

	// Create new library lookup
	_, err := NewUpdateLibraryLookup(rootDir, "", "stable", log.NewNopLogger())
	require.NoError(t, err)
}

func TestCheckOutLatest_withTufRepository(t *testing.T) {
	t.Parallel()

	// Set up an update library
	rootDir := t.TempDir()
	updateDir := DefaultLibraryDirectory(rootDir)
	require.NoError(t, os.MkdirAll(filepath.Join(updateDir, "launcher"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(updateDir, "osqueryd"), 0755))

	// Set up a local TUF repo
	tufDir := LocalTufDirectory(rootDir)
	require.NoError(t, os.MkdirAll(tufDir, 488))
	testReleaseVersion := "1.0.30"
	expectedTargetName := fmt.Sprintf("launcher-%s.tar.gz", testReleaseVersion)
	testRootJson := tufci.SeedLocalTufRepo(t, testReleaseVersion, rootDir)

	// Create the test library lookup
	testLibLookup, err := NewUpdateLibraryLookup(rootDir, "", "stable", log.NewNopLogger())
	require.NoError(t, err)
	require.NotNil(t, testLibLookup.metadataClient)
	mockROLibrary := NewMockreadOnlyUpdateLibrary(t)
	testLibLookup.library = mockROLibrary
	require.NoError(t, testLibLookup.metadataClient.Init(testRootJson), "could not initialize metadata client with test root JSON")

	// Expect that we find the given release and see if it's available in the library
	mockROLibrary.On("IsInstallVersion", binaryLauncher, expectedTargetName).Return(false)
	mockROLibrary.On("Available", binaryLauncher, expectedTargetName).Return(true)
	expectedPath := "some/path/to/the/expected/target"
	mockROLibrary.On("PathToTargetVersionExecutable", binaryLauncher, expectedTargetName).Return(expectedPath)

	// Check it
	latestPath, err := testLibLookup.CheckOutLatest(binaryLauncher)
	require.NoError(t, err, "unexpected error on checking out latest")
	mockROLibrary.AssertExpectations(t)
	require.Equal(t, expectedPath, latestPath)
}

func TestCheckOutLatest_withoutTufRepository(t *testing.T) {
	t.Parallel()

	// Set up an update library, but no TUF repo
	rootDir := t.TempDir()
	updateDir := DefaultLibraryDirectory(rootDir)
	require.NoError(t, os.MkdirAll(filepath.Join(updateDir, "launcher"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(updateDir, "osqueryd"), 0755))

	// Create the test library lookup
	testLibLookup, err := NewUpdateLibraryLookup(rootDir, "", "stable", log.NewNopLogger())
	require.NoError(t, err)
	require.Nil(t, testLibLookup.metadataClient)
	mockROLibrary := NewMockreadOnlyUpdateLibrary(t)
	testLibLookup.library = mockROLibrary

	// Expect that we fall back to picking the most recent update
	expectedPath := "a/path/to/the/update"
	mockROLibrary.On("MostRecentVersion", binaryLauncher).Return(expectedPath, nil)

	// Check it
	latestPath, err := testLibLookup.CheckOutLatest(binaryLauncher)
	require.NoError(t, err, "unexpected error on checking out latest")
	mockROLibrary.AssertExpectations(t)
	require.Equal(t, expectedPath, latestPath)
}

func TestCheckOutLatest_withInstalledVersionIsReleaseVersion(t *testing.T) {
	t.Parallel()

	// Set up an update library
	rootDir := t.TempDir()
	updateDir := DefaultLibraryDirectory(rootDir)
	require.NoError(t, os.MkdirAll(filepath.Join(updateDir, "launcher"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(updateDir, "osqueryd"), 0755))

	// Set up a local TUF repo
	tufDir := LocalTufDirectory(rootDir)
	require.NoError(t, os.MkdirAll(tufDir, 488))
	testReleaseVersion := "1.0.30"
	expectedTargetName := fmt.Sprintf("launcher-%s.tar.gz", testReleaseVersion)
	testRootJson := tufci.SeedLocalTufRepo(t, testReleaseVersion, rootDir)

	// Create the test library lookup
	testLibLookup, err := NewUpdateLibraryLookup(rootDir, "", "stable", log.NewNopLogger())
	require.NoError(t, err)
	require.NotNil(t, testLibLookup.metadataClient)
	mockROLibrary := NewMockreadOnlyUpdateLibrary(t)
	testLibLookup.library = mockROLibrary
	require.NoError(t, testLibLookup.metadataClient.Init(testRootJson), "could not initialize metadata client with test root JSON")

	// Expect that we find the given release and see that it's the installed version
	mockROLibrary.On("IsInstallVersion", binaryLauncher, expectedTargetName).Return(true)

	// Check it -- we expect the path to be "" since it's the installed version
	latestPath, err := testLibLookup.CheckOutLatest(binaryLauncher)
	require.NoError(t, err, "unexpected error on checking out latest")
	mockROLibrary.AssertExpectations(t)
	require.Equal(t, "", latestPath)
}
