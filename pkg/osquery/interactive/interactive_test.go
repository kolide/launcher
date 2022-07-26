package interactive

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kolide/kit/fs"
	"github.com/kolide/launcher/pkg/packaging"
	"github.com/stretchr/testify/require"
)

var osquerydCacheDir = filepath.Join(os.TempDir(), "launcher_interactive_tests")

func TestMain(m *testing.M) {
	// download and cache the osquerd binary before tests run
	target := packaging.Target{}
	if err := target.PlatformFromString(runtime.GOOS); err != nil {
		fmt.Printf("error parsing platform: %s, %s", err, runtime.GOOS)
		os.Exit(1)
	}

	if err := os.MkdirAll(osquerydCacheDir, fs.DirMode); err != nil {
		fmt.Printf("error creating cache dir: %s", err)
		os.Exit(1)
	}

	_, err := packaging.FetchBinary(context.TODO(), osquerydCacheDir, "osqueryd", target.PlatformBinaryName("osqueryd"), "stable", target)
	if err != nil {
		fmt.Printf("error fetching binary osqueryd binary: %s", err)
		os.Exit(1)
	}

	// Run the tests!
	retCode := m.Run()
	os.Exit(retCode)
}

// TestProc tests the start process function, it's named weird because path of the temp dir has to be short enough
// to not exceed the max number of charcters for the socket path.
func TestProc(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		osqueryFlags         []string
		wantProc             bool
		errContainsStr       string
		skipWindowsRationale string
	}{
		{
			name:     "no flags",
			wantProc: true,
		},
		{
			name: "flags",
			osqueryFlags: []string{
				"verbose",
				"force=false",
			},
			wantProc: true,
		},
		{
			name:                 "make socket path to long to test our error handling, making this really long causes the socket path to be long",
			wantProc:             false,
			errContainsStr:       "exceeded the maximum socket path character length",
			skipWindowsRationale: "socket path is always the same on windows, so error can't be tested",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.skipWindowsRationale != "" && runtime.GOOS == "windows" {
				t.Skip(tt.skipWindowsRationale)
			}

			rootDir := t.TempDir()
			require.NoError(t, downloadOsquery(rootDir))
			osquerydPath := filepath.Join(rootDir, "osqueryd")

			proc, _, err := StartProcess(rootDir, osquerydPath, tt.osqueryFlags)

			if tt.errContainsStr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errContainsStr)
			} else {
				require.NoError(t, err)
			}

			if tt.wantProc {
				require.NotNil(t, proc)

				// Wait until proc exits
				_, err := proc.Wait()
				if err != nil {
					require.NoError(t, err)
				}
			} else {
				require.Nil(t, proc)
			}
		})
	}
}

func downloadOsquery(dir string) error {
	target := packaging.Target{}
	if err := target.PlatformFromString(runtime.GOOS); err != nil {
		return fmt.Errorf("error parsing platform: %w, %s", err, runtime.GOOS)
	}

	outputFile := filepath.Join(dir, "osqueryd")

	path, err := packaging.FetchBinary(context.TODO(), osquerydCacheDir, "osqueryd", target.PlatformBinaryName("osqueryd"), "stable", target)
	if err != nil {
		return fmt.Errorf("error fetching binary osqueryd binary: %w", err)
	}

	if err := fs.CopyFile(path, outputFile); err != nil {
		return fmt.Errorf("error copying binary osqueryd binary: %w", err)
	}

	return nil
}
