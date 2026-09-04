package tufci

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/theupdateframework/go-tuf/v2/metadata/config"
	"github.com/theupdateframework/go-tuf/v2/metadata/updater"
)

// SeedLocalTufRepo creates a local TUF repo with a valid release under the given version `testTargetVersion`
func SeedLocalTufRepo(t *testing.T, testTargetVersion string, testRootDir string) {
	serverUrl, testRootJson := InitRemoteTufServer(t, testTargetVersion)

	localTufDir := filepath.Join(testRootDir, "tuf")
	localTargetsDir := filepath.Join(testRootDir, "updates")

	metadataUrl := strings.TrimSuffix(serverUrl, "/") + "/repository"
	cfg, err := config.New(metadataUrl, testRootJson)
	require.NoError(t, err)
	cfg.LocalMetadataDir = localTufDir
	cfg.LocalTargetsDir = localTargetsDir

	up, err := updater.New(cfg)
	require.NoError(t, err, "initializing updater")
	require.NoError(t, up.Refresh(), "refreshing TUF metadata")

	// The old version of go-tuf expects permissions at 0640 at most, but go-tuf/v2
	// creates metadata files at 0644. So, re-chmod any metadata files added by
	// the call to `Refresh`.
	metadataFiles, err := filepath.Glob(filepath.Join(localTufDir, "*.json"))
	require.NoError(t, err)
	for _, metadataFile := range metadataFiles {
		require.NoError(t, os.Chmod(metadataFile, 0640))
	}
}
