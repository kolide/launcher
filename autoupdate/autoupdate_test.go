package autoupdate

import (
	"fmt"
	"net/http"
	"regexp"
	"testing"

	"github.com/kolide/launcher/osquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateTUFRepoDirectory(t *testing.T) {
	td := map[string][]string{
		"root": []string{
			"root.json",
			"snapshot.json",
			"targets.json",
			"timestamp.json",
			"targets",
		},
		"root/targets": []string{
			"releases.json",
			"betas.json",
			"releases",
		},
		"root/targets/releases": []string{
			"zippy.json",
		},
	}
	expected := map[string]string{
		"root/root.json":                   "local/root.json",
		"root/snapshot.json":               "local/snapshot.json",
		"root/targets.json":                "local/targets.json",
		"root/timestamp.json":              "local/timestamp.json",
		"root/targets/releases.json":       "local/targets/releases.json",
		"root/targets/betas.json":          "local/targets/betas.json",
		"root/targets":                     "local/targets",
		"root/targets/releases":            "local/targets/releases",
		"root/targets/releases/zippy.json": "local/targets/releases/zippy.json",
	}
	actual := map[string]string{}
	assetDir := func(path string) ([]string, error) {
		return td[path], nil
	}
	copier := func(source, dest string) (bool, error) {
		actual[source] = dest
		if !regexp.MustCompile(`\.json$`).MatchString(source) {
			return true, nil
		}
		return false, nil
	}

	err := createTUFRepoDirectory("local", "root", assetDir, copier)
	require.Nil(t, err)

	require.Equal(t, len(expected), len(actual))
	for k, v := range expected {
		va, ok := actual[k]
		assert.True(t, ok)
		assert.Equal(t, v, va)
	}
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
			notaryURL:     defaultNotary,
			mirrorURL:     defaultMirror,
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
			require.Nil(t, err)

			assert.Equal(t, tt.target, u.target)

			// check tuf.Settings derived from NewUpdater defaults.
			assert.Equal(t, u.settings.GUN, gun)
			assert.Equal(t, u.settings.LocalRepoPath, tt.localRepoPath)
			assert.Equal(t, u.settings.NotaryURL, tt.notaryURL)
			assert.Equal(t, u.settings.MirrorURL, tt.mirrorURL)

			// must have a non-nil finalizer
			require.NotNil(t, u.finalizer)
			assert.Nil(t, u.finalizer())
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
