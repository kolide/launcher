//go:build darwin
// +build darwin

package timemachine

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/pkg/backoff"
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

	AddExclusions(context.TODO(), knapsack)

	// we've seen some flake in CI here where the exclusions have not been
	// updated by the time we perform assertions, so sleep for a bit to give
	// OS some time to catch up
	time.Sleep(1 * time.Second)

	// ensure the files are included / excluded as expected
	for fileName, shouldBeExcluded := range shouldBeExcluded {
		// Allow for a couple retries for the file to show up as excluded appropriately --
		// we've seen this be a little flaky in CI
		err := backoff.WaitFor(func() error {
			cmd, err := allowedcmd.Tmutil(context.TODO(), "isexcluded", filepath.Join(testDir, fileName))
			if err != nil {
				return fmt.Errorf("creating tmutil command: %w", err)
			}

			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("running tmutil isexcluded: %w", err)
			}

			if shouldBeExcluded && !strings.Contains(string(out), "[Excluded]") {
				return fmt.Errorf("output `%s` does not contain [Excluded], but file should be excluded", string(out))
			}

			if !shouldBeExcluded && !strings.Contains(string(out), "[Included]") {
				return fmt.Errorf("output `%s` does not contain [Included], but file should be included", string(out))
			}

			return nil
		}, 1*time.Second, 200*time.Millisecond)

		require.NoError(t, err, "file not handled as expected")
	}
}
