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

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"
	gotuf "github.com/theupdateframework/go-tuf"
	"github.com/theupdateframework/go-tuf/client"
	filejsonstore "github.com/theupdateframework/go-tuf/client/filejsonstore"
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
	testRootJson := seedTufRepo(t, binaryLauncher, expectedTargetName, rootDir)

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
	testRootJson := seedTufRepo(t, binaryLauncher, expectedTargetName, rootDir)

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

// TODO: this is more or less duplicated with the tufinfo table tests (release_version_test.go) -- we should
// should dedup somewhere
func seedTufRepo(t *testing.T, binary autoupdatableBinary, releaseTarget string, testRootDir string) []byte {
	// Create a "remote" TUF repo and seed it with the expected targets
	tufDir := t.TempDir()

	// Initialize remote repo with store
	fsStore := gotuf.FileSystemStore(tufDir, nil)
	repo, err := gotuf.NewRepo(fsStore)
	require.NoError(t, err, "could not create new tuf repo")

	// Gen keys
	_, err = repo.GenKey("root")
	require.NoError(t, err, "could not gen root key")
	_, err = repo.GenKey("targets")
	require.NoError(t, err, "could not gen targets key")
	_, err = repo.GenKey("snapshot")
	require.NoError(t, err, "could not gen snapshot key")
	_, err = repo.GenKey("timestamp")
	require.NoError(t, err, "could not gen timestamp key")

	// Seed release files
	require.NoError(t, os.MkdirAll(filepath.Join(tufDir, "staged", "targets", string(binary), runtime.GOOS, "stable"), 0777), "could not make staging directory")
	err = os.WriteFile(filepath.Join(tufDir, "staged", "targets", string(binary), runtime.GOOS, "stable", "release.json"), []byte("{}"), 0777)
	require.NoError(t, err, "could not write test target release file to temp dir")
	customMetadata := fmt.Sprintf("{\"target\":\"%s/%s/%s\"}", binary, runtime.GOOS, releaseTarget)
	require.NoError(t, repo.AddTarget(fmt.Sprintf("%s/%s/%s/release.json", binary, runtime.GOOS, "stable"), []byte(customMetadata)), "could not add test target release file to tuf")

	// Seed target file launcher/darwin/launcher-1.0.30.tar.gz
	err = os.WriteFile(filepath.Join(tufDir, "staged", "targets", string(binary), runtime.GOOS, releaseTarget), []byte("{}"), 0777)
	require.NoError(t, err, "could not write test target file to temp dir")
	require.NoError(t, repo.AddTarget(fmt.Sprintf("%s/%s/%s", binary, runtime.GOOS, releaseTarget), nil), "could not add test target release file to tuf")

	require.NoError(t, repo.Snapshot(), "could not take snapshot")
	require.NoError(t, repo.Timestamp(), "could not take timestamp")
	require.NoError(t, repo.Commit(), "could not commit")

	// Quick validation that we set up the repo properly
	require.FileExists(t, filepath.Join(tufDir, "repository", "targets.json"))

	// Set up a httptest server to serve this data to our local repo
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

	// Get metadata to initialize local store
	metadata, err := repo.GetMeta()
	require.NoError(t, err, "could not get metadata")

	// Now set up local repo
	localTufDir := LocalTufDirectory(testRootDir)
	localStore, err := filejsonstore.NewFileJSONStore(localTufDir)
	require.NoError(t, err, "could not set up local store")

	// Set up our remote store i.e. tuf.kolide.com
	remoteOpts := client.HTTPRemoteOptions{
		MetadataPath: "/repository",
	}
	remoteStore, err := client.HTTPRemoteStore(testMetadataServer.URL, &remoteOpts, http.DefaultClient)
	require.NoError(t, err, "could not set up remote store")

	metadataClient := client.NewClient(localStore, remoteStore)
	require.NoError(t, err, metadataClient.Init(metadata["root.json"]), "failed to initialze TUF client")

	_, err = metadataClient.Update()
	require.NoError(t, err, "could not update TUF client")

	return metadata["root.json"]
}
