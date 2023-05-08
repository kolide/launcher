package tufci

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/theupdateframework/go-tuf/client"
	filejsonstore "github.com/theupdateframework/go-tuf/client/filejsonstore"
)

// SeedLocalTufRepo creates a local TUF repo with a valid release under the given version `testTargetVersion`
func SeedLocalTufRepo(t *testing.T, testTargetVersion string, testRootDir string) {
	serverUrl, testRootJson := InitRemoteTufServer(t, testTargetVersion)

	// Now set up local repo
	localTufDir := filepath.Join(testRootDir, "tuf")
	localStore, err := filejsonstore.NewFileJSONStore(localTufDir)
	require.NoError(t, err, "could not set up local store")

	// Set up our remote store i.e. tuf.kolide.com
	remoteOpts := client.HTTPRemoteOptions{
		MetadataPath: "/repository",
	}
	remoteStore, err := client.HTTPRemoteStore(serverUrl, &remoteOpts, http.DefaultClient)
	require.NoError(t, err, "could not set up remote store")

	metadataClient := client.NewClient(localStore, remoteStore)
	require.NoError(t, err, metadataClient.Init(testRootJson), "failed to initialze TUF client")

	_, err = metadataClient.Update()
	require.NoError(t, err, "could not update TUF client")
}
