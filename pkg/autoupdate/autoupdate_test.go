package autoupdate

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/kolide/launcher/pkg/osquery"
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

func TestFindCurrentUpdate(t *testing.T) {
	t.Parallel()

	// Setup the tests
	tmpDir, err := ioutil.TempDir("", "test-autoupdate-findCurrentUpdate")
	defer os.RemoveAll(tmpDir)
	require.NoError(t, err)

	updater := Updater{
		binaryName:       "binary",
		updatesDirectory: tmpDir,
		logger:           log.NewNopLogger(),
	}

	// test with empty directory
	require.Equal(t, updater.findCurrentUpdate(), "", "No subdirs, nothing should be found")

	// make some (still empty) test directories
	for _, n := range []string{"2", "5", "3", "1"} {
		require.NoError(t, os.Mkdir(filepath.Join(tmpDir, n), 0755))
	}

	// empty directories -- should still be none
	require.Equal(t, updater.findCurrentUpdate(), "", "Empty directories should not be found")

	for _, n := range []string{"2", "5", "3", "1"} {
		f := filepath.Join(tmpDir, n, "binary")
		if runtime.GOOS == "windows" {
			f = f + ".exe"
		}
		require.NoError(t, copyFile(f, os.Args[0], false), "copy executable")
	}

	// Windows doesn't have an executable bit, so skip some tests.
	if runtime.GOOS == "windows" {
		require.Equal(t, updater.findCurrentUpdate(), filepath.Join(tmpDir, "5", "binary.exe"), "Should find number 5")
	}

	// Nothing executable -- should still be none
	require.Equal(t, updater.findCurrentUpdate(), "", "Non-executable files should not be found")

	//
	// Chmod some of them
	//

	require.NoError(t, os.Chmod(filepath.Join(tmpDir, "1", "binary"), 0755))
	require.Equal(t, updater.findCurrentUpdate(), filepath.Join(tmpDir, "1", "binary"), "Should find number 1")

	for _, n := range []string{"2", "5", "3", "1"} {
		require.NoError(t, os.Chmod(filepath.Join(tmpDir, n, "binary"), 0755))
	}
	require.Equal(t, updater.findCurrentUpdate(), filepath.Join(tmpDir, "5", "binary"), "Should find number 5")

}
