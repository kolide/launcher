package autoupdate

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateTUFRepoDirectory(t *testing.T) {
	localTUFRepoPath, err := ioutil.TempDir("", "")
	require.NoError(t, err)

	u := &Updater{}
	require.NoError(t, u.createTUFRepoDirectory(localTUFRepoPath, "pkg/autoupdate/assets", AssetDir))

	knownFilePaths := []string{
		"launcher-tuf/root.json",
		"launcher-tuf/snapshot.json",
		"launcher-tuf/targets.json",
		"launcher-tuf/timestamp.json",
		"launcher-tuf/targets/releases.json",
		"osqueryd-tuf/root.json",
		"osqueryd-tuf/snapshot.json",
		"osqueryd-tuf/targets.json",
		"osqueryd-tuf/timestamp.json",
		"osqueryd-tuf/targets/releases.json",
	}

	for _, knownFilePath := range knownFilePaths {
		_, err = os.Stat(filepath.Join(localTUFRepoPath, knownFilePath))
		require.NoError(t, err)
	}

	require.NoError(t, os.RemoveAll(localTUFRepoPath))
}

func TestNewUpdater(t *testing.T) {
	var tests = []struct {
		name          string
		opts          []UpdaterOption
		httpClient    *http.Client
		target        string
		localRepoPath string
		notaryURL     string
		mirrorURL     string
	}{
		{
			name:          "default",
			opts:          nil,
			httpClient:    http.DefaultClient,
			target:        withPlatform(t, "%s/app-stable.tar.gz"),
			localRepoPath: "/tmp/tuf/app-tuf",
			notaryURL:     DefaultNotary,
			mirrorURL:     DefaultMirror,
		},
		{
			name: "with-opts",
			opts: []UpdaterOption{
				WithHTTPClient(nil),
				WithUpdateChannel(Beta),
				WithNotaryURL("https://notary"),
				WithMirrorURL("https://mirror"),
				WithGUNPrefix("kolide/"),
			},
			httpClient:    nil,
			target:        withPlatform(t, "%s/app-beta.tar.gz"),
			localRepoPath: "/tmp/tuf/app-tuf",
			notaryURL:     "https://notary",
			mirrorURL:     "https://mirror",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gun := fmt.Sprintf("kolide/app")
			tt.opts = append(tt.opts, withoutBootstrap())
			u, err := NewUpdater("/tmp/app", "/tmp/tuf", log.NewNopLogger(), tt.opts...)
			require.NoError(t, err)

			assert.Equal(t, tt.target, u.target)

			// check tuf.Settings derived from NewUpdater defaults.
			assert.Equal(t, u.settings.GUN, gun)
			assert.Equal(t, u.settings.LocalRepoPath, tt.localRepoPath)
			assert.Equal(t, u.settings.NotaryURL, tt.notaryURL)
			assert.Equal(t, u.settings.MirrorURL, tt.mirrorURL)

			// must have a non-nil finalizer
			require.NotNil(t, u.finalizer)

			// Running finalizer shouldn't error
			require.NoError(t, u.finalizer())
		})
	}
}

func withPlatform(t *testing.T, format string) string {
	platform, err := osquery.DetectPlatform()
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf(format, platform)
}
