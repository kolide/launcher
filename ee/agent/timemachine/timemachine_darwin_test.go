//go:build darwin
// +build darwin

package timemachine

import (
	"context"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/stretchr/testify/require"
)

func TestAddExclusions(t *testing.T) {
	t.Parallel()

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
		"augeas-lenses/":                        true,
		"debug-2024-01-09T21-15-14.055.json.gz": true,
		"debug.json":                            true,
		"desktop_501/":                          true,
		"desktop_503/":                          true,
		"kv.sqlite":                             true,
		"launcher.db":                           true,
		"launcher.pid":                          true,
		"menu.json":                             true,
		"menu_template.json":                    true,
		"metadata.json":                         true,
		"metadata.plist":                        true,
		"osquery.autoload":                      true,
		"osquery.db/":                           true,
		"osquery.pid":                           true,
		"osquery.sock":                          true,
		"osquery.sock.51807":                    true,
		"osquery.sock.63071":                    true,
		"osqueryd-tuf/":                         true,

		// these should NOT be excluded
		"launcher-tuf/":                     false,
		"tuf/":                              false,
		"updates/":                          false,
		"kolide.png":                        false,
		"launcher-version-1.4.1-4-gdb7106f": false,
	}

	// create files and dirs
	for k := range shouldBeExcluded {
		path := filepath.Join(testDir, k)

		if strings.HasSuffix(k, "/") {
			require.NoError(t, os.MkdirAll(path, 0755))
		} else {
			f, err := os.Create(path)
			require.NoError(t, err)
			f.Close()
		}
	}

	k := mocks.NewKnapsack(t)
	k.On("RootDirectory").Return(testDir)

	AddExclusions(context.TODO(), k)

	// ensure the files are included / excluded as expected
	for fileName, shouldBeExcluded := range shouldBeExcluded {
		cmd, err := allowedcmd.Tmutil(context.TODO(), "isexcluded", filepath.Join(testDir, fileName))
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
