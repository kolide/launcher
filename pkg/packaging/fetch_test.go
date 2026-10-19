package packaging

import (
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"

	tufci "github.com/kolide/launcher/v2/ee/tuf/ci"
	"github.com/stretchr/testify/require"
)

func Test_fetchBinaryFromSource(t *testing.T) {
	t.Parallel()

	testReleaseVersion := "1.2.3"

	for _, tt := range []struct {
		name             string
		channelOrVersion string
	}{
		{name: "channel", channelOrVersion: "stable"},
		{name: "version", channelOrVersion: testReleaseVersion},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			serverURL, testRootJson := tufci.InitRemoteTufServer(t, testReleaseVersion)

			binaryPath, err := fetchBinaryFromSource(t.Context(), serverURL, serverURL, testRootJson, t.TempDir(), "osqueryd", testBinaryName(), tt.channelOrVersion, testTarget())
			require.NoError(t, err)
			_, err = os.Stat(binaryPath)
			require.NoError(t, err)
			require.Contains(t, binaryPath, tt.channelOrVersion)
		})
	}
}

func Test_fetchBinaryFromSource_UsesCache(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	serverURL, testRootJson := tufci.InitRemoteTufServer(t, "2.5.0")

	firstPath, err := fetchBinaryFromSource(t.Context(), serverURL, serverURL, testRootJson, cacheDir, "osqueryd", testBinaryName(), "stable", testTarget())
	require.NoError(t, err)

	// Confirm we use the cache -- offlineURL is a server that won't return
	// any data, so if fetchBinaryFromSource succeeds, it's pulling from cache.
	offlineURL := offlineServerURL(t)
	secondPath, err := fetchBinaryFromSource(t.Context(), offlineURL, offlineURL, testRootJson, cacheDir, "osqueryd", testBinaryName(), "stable", testTarget())
	require.NoError(t, err)
	require.Equal(t, firstPath, secondPath)
}

func Test_getReleaseVersionFromTufRepo(t *testing.T) {
	t.Parallel()

	testReleaseVersion := "2.0.0"

	for _, channel := range []string{"stable", "beta", "alpha", "nightly"} {
		t.Run(channel, func(t *testing.T) {
			t.Parallel()

			serverURL, testRootJson := tufci.InitRemoteTufServer(t, testReleaseVersion)
			target := testTarget()

			version, err := getReleaseVersionFromTufRepo(serverURL, testRootJson, "osqueryd", channel, string(target.Platform), string(target.Arch), t.TempDir())
			require.NoError(t, err)
			require.Equal(t, testReleaseVersion, version)
		})
	}
}

func Test_getReleaseVersionFromTufRepo_UsesCachedMetadata(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	target := testTarget()
	testReleaseVersion := "2.6.0"
	serverURL, testRootJson := tufci.InitRemoteTufServer(t, testReleaseVersion)

	_, err := getReleaseVersionFromTufRepo(serverURL, testRootJson, "osqueryd", "stable", string(target.Platform), string(target.Arch), cacheDir)
	require.NoError(t, err)

	version, err := getReleaseVersionFromTufRepo(offlineServerURL(t), testRootJson, "osqueryd", "stable", string(target.Platform), string(target.Arch), cacheDir)
	require.NoError(t, err)
	require.Equal(t, testReleaseVersion, version)
}

// offlineServerURL returns the URL of a server that fails the test if it receives any request.
func offlineServerURL(t *testing.T) string {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request to %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func testTarget() Target {
	arch := ArchFlavor(runtime.GOARCH)
	if runtime.GOOS == "darwin" {
		arch = Universal
	}
	return Target{Platform: PlatformFlavor(runtime.GOOS), Arch: arch}
}

func testBinaryName() string {
	switch runtime.GOOS {
	case "darwin":
		return "osquery.app"
	case "windows":
		return "osqueryd.exe"
	default:
		return "osqueryd"
	}
}
