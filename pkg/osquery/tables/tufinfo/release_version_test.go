package tufinfo

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kolide/launcher/pkg/autoupdate/tuf"
	"github.com/kolide/launcher/pkg/launcher"
	"github.com/osquery/osquery-go/gen/osquery"
	"github.com/stretchr/testify/require"
	gotuf "github.com/theupdateframework/go-tuf"
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

	// Call table generate func and validate that our data matches what exists in the filesystem
	testTable := TufReleaseVersionTable(&launcher.Options{RootDirectory: testRootDir})
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
	// Create a TUF repo and seed it with the expected targets
	tufDir := tuf.LocalTufDirectory(testRootDir, binary)

	// Initialize repo with store
	localStore := gotuf.FileSystemStore(tufDir, nil)
	repo, err := gotuf.NewRepo(localStore)
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
