package tufinfo

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kolide/launcher/pkg/agent/types/mocks"
	"github.com/kolide/launcher/pkg/autoupdate/tuf"
	"github.com/osquery/osquery-go/gen/osquery"

	"github.com/stretchr/testify/require"
	gotuf "github.com/theupdateframework/go-tuf"
	"github.com/theupdateframework/go-tuf/client"
	filejsonstore "github.com/theupdateframework/go-tuf/client/filejsonstore"
)

type tufTarget struct {
	binary          string
	operatingSystem string
	channel         string
	target          string
}

func TestTufReleaseVersionTable(t *testing.T) {
	t.Parallel()

	rand.Seed(time.Now().UnixNano())

	// Set up some expected results
	testTargets := make([]tufTarget, 0)
	expectedResults := make(map[string]map[string]map[string]string, 0)

	for _, binary := range []string{"launcher", "osqueryd"} {
		expectedResults[binary] = make(map[string]map[string]string, 0)
		for _, operatingSystem := range []string{"darwin", "windows", "linux"} {
			expectedResults[binary][operatingSystem] = make(map[string]string, 0)
			for _, channel := range []string{"stable", "beta", "nightly"} {
				testTarget := fmt.Sprintf("%s-%s.tar.gz", binary, randomSemver())
				testTargets = append(testTargets, tufTarget{
					binary:          binary,
					operatingSystem: operatingSystem,
					channel:         channel,
					target:          testTarget,
				})
				expectedResults[binary][operatingSystem][channel] = fmt.Sprintf("%s/%s/%s", binary, operatingSystem, testTarget)
			}
		}
	}

	// Seed our test repos with expected targets
	testRootDir := t.TempDir()
	seedTufRepo(t, testTargets, testRootDir, "launcher")
	seedTufRepo(t, testTargets, testRootDir, "osqueryd")

	mockFlags := mocks.NewFlags(t)
	mockFlags.On("RootDirectory").Return(testRootDir)

	// Call table generate func and validate that our data matches what exists in the filesystem
	testTable := TufReleaseVersionTable(mockFlags)
	resp := testTable.Call(context.Background(), osquery.ExtensionPluginRequest{
		"action":  "generate",
		"context": "{}",
	})

	// Require success
	require.Equal(t, int32(0), resp.Status.Code, fmt.Sprintf("unexpected failure generating table: %s", resp.Status.Message))

	// Require results
	require.Greater(t, len(resp.Response), 0, "expected results but did not receive any")

	for _, row := range resp.Response {
		expectedTarget, ok := expectedResults[row["binary"]][row["operating_system"]][row["channel"]]
		require.True(t, ok, "found unexpected row: %v", row)
		require.Equal(t, expectedTarget, row["target"], "target mismatch")
	}
}

func seedTufRepo(t *testing.T, testTargets []tufTarget, testRootDir string, binary string) {
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
	for _, testTarget := range testTargets {
		require.NoError(t, os.MkdirAll(filepath.Join(tufDir, "staged", "targets", testTarget.binary, testTarget.operatingSystem, testTarget.channel), 0777), "could not make staging directory")
		err = os.WriteFile(filepath.Join(tufDir, "staged", "targets", testTarget.binary, testTarget.operatingSystem, testTarget.channel, "release.json"), []byte("{}"), 0777)
		require.NoError(t, err, "could not write test target release file to temp dir")
		customMetadata := fmt.Sprintf("{\"target\":\"%s/%s/%s\"}", testTarget.binary, testTarget.operatingSystem, testTarget.target)
		require.NoError(t, repo.AddTarget(fmt.Sprintf("%s/%s/%s/release.json", testTarget.binary, testTarget.operatingSystem, testTarget.channel), []byte(customMetadata)), "could not add test target release file to tuf")

		require.NoError(t, repo.Snapshot(), "could not take snapshot")
		require.NoError(t, repo.Timestamp(), "could not take timestamp")
		require.NoError(t, repo.Commit(), "could not commit")
	}

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
	localTufDir := tuf.LocalTufDirectory(testRootDir)
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

func randomSemver() string {
	major := rand.Intn(100) - 1
	minor := rand.Intn(100) - 1
	patch := rand.Intn(100) - 1

	// Only sometimes include hash
	includeHash := rand.Int()
	if includeHash%2 == 0 {
		return fmt.Sprintf("%d.%d.%d", major, minor, patch)
	}

	randUuid := uuid.New().String()

	return fmt.Sprintf("%d.%d.%d-%s", major, minor, patch, randUuid[0:8])
}
