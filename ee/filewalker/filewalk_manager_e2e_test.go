package filewalker

import (
	"encoding/binary"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/kolide/launcher/v2/ee/agent/storage"
	storageci "github.com/kolide/launcher/v2/ee/agent/storage/ci"
	typesmocks "github.com/kolide/launcher/v2/ee/agent/types/mocks"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

// Verifies broad behavior through public APIs of the package against a prod-like configuration.
// Checks results from calling public APIs: a startup scan, Do() in two ways, and a Ping().
func TestFilewalkManager_E2E(t *testing.T) {
	t.Parallel()

	configs := `{
  "dotenv_files": {
    "walk_interval": "2h",
    "root_dirs": ["$ROOT/home/*"],
    "file_name_regex": "^(.+\\.env|\\.env(\\..+)?)$",
    "skip_dirs": ["node_modules$", "vendor$", "\\.cache$", "Library$", "\\.git$"],
    "file_type_filter": "file"
  },
  "git_repos": {
    "walk_interval": "2h",
    "root_dirs": ["$ROOT/home/*"],
    "file_name_regex": "^\\.git$",
    "skip_dirs": ["node_modules$", "vendor$", "\\.cache$", "Library$", "\\.git/"],
    "file_type_filter": "dir"
  },
  "secret_files": {
    "walk_interval": "4h",
    "root_dirs": ["$ROOT/home/*"],
    "file_name_regex": "^(?:\\.env(\\..+)?|.+\\.env|.+\\.tfvars|\\.netrc|\\.git-credentials|\\.npmrc|\\.pgpass|credentials(\\.json)?)$",
    "skip_dirs": ["node_modules$", "vendor$", "\\.cache$", "Library$", "\\.git$", "Temp$"],
    "file_type_filter": "file"
  },
  "host_keys": {
    "walk_interval": "4h",
    "root_dirs": ["$ROOT/etc/ssh"],
    "file_name_regex": "^ssh_host_.+_key$",
    "file_type_filter": "file"
  },
  "shell_history": {
    "walk_interval": "24h",
    "root_dirs": ["$ROOT/home/*"],
    "file_name_regex": "^\\.(bash|zsh)_history$",
    "skip_dirs": ["node_modules$", "\\.cache$", "\\.git$"],
    "file_type_filter": "file"
  }
}`

	fsys := fstest.MapFS{
		"etc/hosts":                                             {},
		"etc/ssh/sshd_config":                                   {},
		"etc/ssh/ssh_host_ed25519_key":                          {},
		"etc/ssh/ssh_host_ed25519_key.pub":                      {},
		"home/kiwi/.netrc":                                      {},
		"home/kiwi/.git-credentials":                            {},
		"home/kiwi/.zsh_history":                                {},
		"home/kiwi/Documents/gcp/credentials.json":              {},
		"home/kiwi/projects/api/.env":                           {},
		"home/kiwi/projects/api/.env.local":                     {},
		"home/kiwi/projects/api/.git/HEAD":                      {},
		"home/kiwi/projects/api/.git/config":                    {},
		"home/kiwi/projects/api/main.go":                        {},
		"home/kiwi/projects/api/deploy/prod.tfvars":             {},
		"home/kiwi/projects/api/node_modules/dotenv/tests/.env": {},
		"home/kiwi/projects/web/.git/HEAD":                      {},
		"home/kiwi/projects/web/package.json":                   {},
		"home/kiwi/projects/web/node_modules/react/index.js":    {},
		"home/ostrich/.npmrc":                                   {},
		"home/ostrich/.bash_history":                            {},
		"home/ostrich/.aws/credentials":                         {},
		"home/ostrich/.cache/pre-commit/repo1/.env":             {},
		"home/ostrich/work/infra/.env":                          {},
		"home/ostrich/work/infra/.git/HEAD":                     {},
		"home/ostrich/work/infra/terraform.tfvars":              {},
	}

	notDue := map[string]time.Duration{
		"shell_history": 1 * time.Hour,
	}

	expected := map[string][]string{
		"dotenv_files": {
			"home/kiwi/projects/api/.env",
			"home/kiwi/projects/api/.env.local",
			"home/ostrich/work/infra/.env",
		},
		"git_repos": {
			"home/kiwi/projects/api/.git",
			"home/kiwi/projects/web/.git",
			"home/ostrich/work/infra/.git",
		},
		"secret_files": {
			"home/kiwi/.netrc",
			"home/kiwi/.git-credentials",
			"home/kiwi/Documents/gcp/credentials.json",
			"home/kiwi/projects/api/.env",
			"home/kiwi/projects/api/.env.local",
			"home/kiwi/projects/api/deploy/prod.tfvars",
			"home/ostrich/.npmrc",
			"home/ostrich/.aws/credentials",
			"home/ostrich/work/infra/.env",
			"home/ostrich/work/infra/terraform.tfvars",
		},
		"host_keys": {
			"etc/ssh/ssh_host_ed25519_key",
		},
		"shell_history": {
			"home/kiwi/.zsh_history",
			"home/ostrich/.bash_history",
		},
	}

	root := t.TempDir()
	require.NoError(t, os.CopyFS(root, fsys))

	slogger := multislogger.NewNopLogger()
	cfgStore, err := storageci.NewStore(t, slogger, storage.FilewalkConfigStore.String())
	require.NoError(t, err)
	resultsStore, err := storageci.NewStore(t, slogger, storage.FilewalkResultsStore.String())
	require.NoError(t, err)
	k := typesmocks.NewKnapsack(t)
	k.On("FilewalkConfigStore").Return(cfgStore)
	k.On("FilewalkResultsStore").Return(resultsStore)

	escapedRoot, err := json.Marshal(root)
	require.NoError(t, err)
	var rawCfgs map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(strings.ReplaceAll(configs, "$ROOT", strings.Trim(string(escapedRoot), `"`))), &rawCfgs))
	for name, rawCfg := range rawCfgs {
		require.NoError(t, cfgStore.Set([]byte(name), rawCfg))
	}

	// sets the last walk to the provided time
	seedLastWalk := func(name string, ago time.Duration) int64 {
		t.Helper()
		ts := time.Now().Add(-ago).Unix()
		require.NoError(t, resultsStore.Set(LastWalkTimeKey(name), binary.NativeEndian.AppendUint64(nil, uint64(ts))))
		return ts
	}
	// gets the last walk time by walk name
	lastWalkTime := func(name string) int64 {
		raw, err := resultsStore.Get(LastWalkTimeKey(name))
		if err != nil || raw == nil {
			return 0
		}
		return int64(binary.NativeEndian.Uint64(raw))
	}
	// retrieves all stored results for the walk by name
	storedResults := func(name string) []string {
		t.Helper()
		raw, err := resultsStore.Get([]byte(name))
		require.NoError(t, err)
		var paths []string
		require.NoError(t, json.Unmarshal(raw, &paths))
		for i, path := range paths {
			relPath, err := filepath.Rel(root, path)
			require.NoError(t, err)
			paths[i] = filepath.ToSlash(relPath)
		}
		return paths
	}
	// assert all named walks ran since the timestamp
	requireWalked := func(names []string, since int64) {
		t.Helper()
		require.Eventually(t, func() bool {
			for _, name := range names {
				if lastWalkTime(name) < since {
					return false
				}
			}
			return true
		}, 10*time.Second, 50*time.Millisecond)
		for _, name := range names {
			require.ElementsMatch(t, expected[name], storedResults(name), name)
		}
	}
	// sets the last walk time for all runs to an hour ago, then returns the
	// values
	seedAll := func() map[string]int64 {
		t.Helper()
		seeded := make(map[string]int64, len(expected))
		for name := range expected {
			seeded[name] = seedLastWalk(name, time.Hour)
		}
		return seeded
	}
	allNames := slices.Collect(maps.Keys(expected))

	for name, ago := range notDue {
		seedLastWalk(name, ago)
	}

	// runs on startup
	walkStart := time.Now().Unix()
	fm := New(k, slogger)
	go fm.Execute()
	t.Cleanup(func() { fm.Interrupt(nil) })
	requireWalked(allNames, walkStart)

	// runs on ping or direct call
	seedAll()
	pingStart := time.Now().Unix()
	fm.Ping()
	requireWalked(allNames, pingStart)

	seedAll()
	doStart := time.Now().Unix()
	require.NoError(t, fm.Do(strings.NewReader(`{"filewalks": []}`)))
	requireWalked(allNames, doStart)

	// and runs with a specific set of walks
	seeded := seedAll()
	doStart = time.Now().Unix()
	require.NoError(t, fm.Do(strings.NewReader(`{"filewalks": ["host_keys"]}`)))
	requireWalked([]string{"host_keys"}, doStart)
	for name, ts := range seeded {
		if name != "host_keys" {
			require.Equal(t, ts, lastWalkTime(name), name)
		}
	}
}
