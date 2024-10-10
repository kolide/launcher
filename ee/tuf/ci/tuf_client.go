package tufci

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/theupdateframework/go-tuf/v2/metadata/config"
	"github.com/theupdateframework/go-tuf/v2/metadata/updater"
)

// SeedLocalTufRepo creates a local TUF repo with a valid release under the given version `testTargetVersion`
func SeedLocalTufRepo(t *testing.T, testTargetVersion string, testRootDir string) []byte {
	serverUrl, testRootJson := InitRemoteTufServer(t, testTargetVersion)

	// Now set up local repo
	localTufDir := filepath.Join(testRootDir, "tuf")
	require.NoError(t, os.MkdirAll(localTufDir, 0750))
	require.NoError(t, os.Chmod(localTufDir, 0750))

	cfg, err := config.New(fmt.Sprintf("%s/repository", serverUrl), testRootJson)
	require.NoError(t, err)
	cfg.LocalMetadataDir = localTufDir
	cfg.LocalTargetsDir = localTufDir // This is unused, but must be set to avoid validation errors when creating the updater below
	client, err := updater.New(cfg)
	require.NoError(t, err)
	require.NoError(t, client.Refresh())

	return testRootJson
}
