package tufci

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	gotuf "github.com/theupdateframework/go-tuf"
	"github.com/theupdateframework/go-tuf/client"
	filejsonstore "github.com/theupdateframework/go-tuf/client/filejsonstore"
)

// SeedLocalTufRepo creates a local TUF repo with a valid release given by `testTarget`
func SeedLocalTufRepo(t *testing.T, testTarget string, channel string, binary string, testRootDir string) {
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
	require.NoError(t, os.MkdirAll(filepath.Join(tufDir, "staged", "targets", string(binary), runtime.GOOS, channel), 0777), "could not make staging directory")
	err = os.WriteFile(filepath.Join(tufDir, "staged", "targets", string(binary), runtime.GOOS, channel, "release.json"), []byte("{}"), 0777)
	require.NoError(t, err, "could not write test target release file to temp dir")
	customMetadata := fmt.Sprintf("{\"target\":\"%s/%s/%s\"}", binary, runtime.GOOS, testTarget)
	require.NoError(t, repo.AddTarget(fmt.Sprintf("%s/%s/%s/release.json", binary, runtime.GOOS, channel), []byte(customMetadata)), "could not add test target release file to tuf")

	// Seed target file
	err = os.WriteFile(filepath.Join(tufDir, "staged", "targets", string(binary), runtime.GOOS, testTarget), []byte("{}"), 0777)
	require.NoError(t, err, "could not write test target file to temp dir")
	require.NoError(t, repo.AddTarget(fmt.Sprintf("%s/%s/%s", binary, runtime.GOOS, testTarget), nil), "could not add test target release file to tuf")

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
	localTufDir := filepath.Join(testRootDir, "tuf")
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
}
