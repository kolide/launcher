//go:build darwin
// +build darwin

package timemachine

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/stretchr/testify/require"
)

func TestAddExclusions(t *testing.T) {
	t.Parallel()

	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("Skipping test on GitHub Actions because it's flaky there")
	}

	u, err := user.Current()
	require.NoError(t, err)

	// time machine will not include temp dirs, so use home dir to test
	// and clean up when done
	testDir := filepath.Join(u.HomeDir, "launcher_tmutil_test_delete_me")
	require.NoError(t, os.RemoveAll(testDir))

	require.NoError(t, os.MkdirAll(testDir, 0755))
	defer os.RemoveAll(testDir)

	shouldBeExcluded := map[string]bool{
		// these should be excluded
		"augeas-lenses/":     true,
		"debug.json":         true,
		"desktop_501/":       true,
		"desktop_503/":       true,
		"kv.sqlite":          true,
		"launcher.db":        true,
		"launcher.db.bak":    true,
		"launcher.db.bak.1":  true,
		"launcher.db.bak.2":  true,
		"launcher.db.bak.3":  true,
		"launcher.pid":       true,
		"menu.json":          true,
		"menu_template.json": true,
		"metadata.json":      true,
		"metadata.plist":     true,
		"osquery.autoload":   true,
		"osquery.db/":        true,
		fmt.Sprintf("osquery-%s.db/", ulid.New()): true,
		"osquery.pid": true,
		fmt.Sprintf("osquery-%s.pid", ulid.New()): true,
		"osquery.sock": true,
		fmt.Sprintf("osquery-%s.sock", ulid.New()): true,
		"osquery.sock.51807":                       true,
		"osquery.sock.63071":                       true,
		"osqueryd-tuf/":                            true,

		// these should NOT be excluded
		"launcher-tuf/":                         false,
		"tuf/":                                  false,
		"updates/":                              false,
		"kolide.png":                            false,
		"launcher-version-1.4.1-4-gdb7106f":     false,
		"debug-2024-01-09T21-15-14.055.json.gz": false,
	}

	// create files and dirs
	for filename := range shouldBeExcluded {
		path := filepath.Join(testDir, filename)

		// create dir
		if strings.HasSuffix(filename, "/") {
			require.NoError(t, os.MkdirAll(path, 0755))
			continue
		}

		// create file
		f, err := os.Create(path)
		require.NoError(t, err)
		f.Close()
	}

	knapsack := mocks.NewKnapsack(t)
	knapsack.On("RootDirectory").Return(testDir)

	AddExclusions(t.Context(), knapsack)

	// we've seen some flake in CI here where the exclusions have not been
	// updated by the time we perform assertions, so sleep for a bit to give
	// OS some time to catch up
	time.Sleep(1 * time.Second)

	// ensure the files are included / excluded as expected
	for fileName, shouldBeExcluded := range shouldBeExcluded {
		cmd, err := allowedcmd.Tmutil(t.Context(), "isexcluded", filepath.Join(testDir, fileName))
		require.NoError(t, err)

		out, err := cmd.CombinedOutput()
		require.NoError(t, err)

		if shouldBeExcluded {
			require.Contains(t, string(out), "[Excluded]")
			continue
		}

		// should be included
		require.Contains(t, string(out), "[Included]")
	}
}
