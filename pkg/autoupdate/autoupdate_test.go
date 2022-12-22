//nolint:typecheck // parts of this come from bindata, so lint fails
package autoupdate

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/stretchr/testify/require"
)

func TestCreateTUFRepoDirectory(t *testing.T) {
	t.Parallel()

	localTUFRepoPath := t.TempDir()

	u := &Updater{logger: log.NewNopLogger()}
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
		fullFilePath := filepath.Join(localTUFRepoPath, knownFilePath)
		_, err := os.Stat(fullFilePath)
		require.NoError(t, err, "stat file")

		jsonBytes, err := os.ReadFile(fullFilePath)
		require.NoError(t, err, "read file")

		require.True(t, json.Valid(jsonBytes), "file is json")
	}

	// Corrupt some local files
	require.NoError(t,
		os.Remove(filepath.Join(localTUFRepoPath, knownFilePaths[0])),
		"remove a tuf file")
	require.NoError(t,
		os.WriteFile(filepath.Join(localTUFRepoPath, knownFilePaths[1]), nil, 0644),
		"truncate a tuf file")

	// Attempt to re-create
	require.NoError(t, u.createTUFRepoDirectory(localTUFRepoPath, "pkg/autoupdate/assets", AssetDir))

	// And retest
	for _, knownFilePath := range knownFilePaths {
		fullFilePath := filepath.Join(localTUFRepoPath, knownFilePath)
		_, err := os.Stat(fullFilePath)
		require.NoError(t, err, "stat file")

		jsonBytes, err := os.ReadFile(fullFilePath)
		require.NoError(t, err, "read file")

		require.True(t, json.Valid(jsonBytes), "file is json")
	}

	require.NoError(t, os.RemoveAll(localTUFRepoPath))
}

func TestValidLocalFile(t *testing.T) {
	t.Parallel()
	var tests = []struct {
		name      string
		content   []byte
		assertion require.BoolAssertionFunc
		logCount  int
	}{
		{
			name:      "no file",
			assertion: require.False,
		},
		{
			name:      "empty",
			content:   []byte{},
			assertion: require.False,
			logCount:  1,
		},

		{
			name:      "space",
			content:   []byte(" "),
			assertion: require.False,
			logCount:  1,
		},
		{
			name:      "dangle brace",
			content:   []byte("{"),
			assertion: require.False,
			logCount:  1,
		},
		{
			name:      "unquoted",
			content:   []byte("{a: 1}"),
			assertion: require.False,
			logCount:  1,
		},
		{
			name:      "valid",
			content:   []byte("{}"),
			assertion: require.True,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testFile, err := os.CreateTemp("", "TestValidLocalFile")
			require.NoError(t, err)
			defer os.Remove(testFile.Name())

			if tt.content == nil {
				require.NoError(t, testFile.Close())
				require.NoError(t, os.Remove(testFile.Name()))
			} else {
				if len(tt.content) > 0 {
					_, err := testFile.Write(tt.content)
					require.NoError(t, err)
				}
				require.NoError(t, testFile.Close())
			}

			l := &mockLogger{}
			u := &Updater{logger: l}
			tt.assertion(t, u.validLocalFile(testFile.Name()))
			require.Equal(t, tt.logCount, l.Count(), "log count")
		})
	}

}

func TestNewUpdater(t *testing.T) {
	t.Parallel()
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
			},
			httpClient:    nil,
			target:        withPlatform(t, "%s/app-beta.tar.gz"),
			localRepoPath: "/tmp/tuf/app-tuf",
			notaryURL:     "https://notary",
			mirrorURL:     "https://mirror",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gun := fmt.Sprintf("kolide/app")
			tt.opts = append(tt.opts, withoutBootstrap())
			u, err := NewUpdater("/tmp/app", "/tmp/tuf", tt.opts...)
			require.NoError(t, err)

			require.Equal(t, tt.target, u.target)

			// check tuf.Settings derived from NewUpdater defaults.
			require.Equal(t, gun, u.settings.GUN)
			require.Equal(t, filepath.Clean(tt.localRepoPath), u.settings.LocalRepoPath)
			require.Equal(t, tt.notaryURL, u.settings.NotaryURL)
			require.Equal(t, tt.mirrorURL, u.settings.MirrorURL)

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
