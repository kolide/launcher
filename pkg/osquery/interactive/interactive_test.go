//go:build !windows
// +build !windows

// disabling on windows because for some reason the test cannot get access to the windows pipe it fails with:
// however it's just the test, works when using interactive mode on windows
// open \\.\pipe\kolide-osquery-.....: The system cannot find the file specified.
package interactive

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kolide/kit/fsutil"
	"github.com/kolide/kit/ulid"
	"github.com/kolide/launcher/ee/agent/types/mocks"
	"github.com/kolide/launcher/pkg/log/multislogger"
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

	if err := os.MkdirAll(osquerydCacheDir, fsutil.DirMode); err != nil {
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
		name           string
		osqueryFlags   []string
		wantProc       bool
		errContainsStr string
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
			name: "config path",
			osqueryFlags: []string{
				fmt.Sprintf("config_path=%s", ulid.New()),
			},
			wantProc: true,
		},
		{
			name:           "socket path too long, the name of the test causes the socket path to be to long to be created, resulting in timeout waiting for the socket",
			wantProc:       false,
			errContainsStr: "error waiting for osquery to create socket",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rootDir := t.TempDir()
			require.NoError(t, downloadOsquery(rootDir))

			mockSack := mocks.NewKnapsack(t)
			mockSack.On("OsquerydPath").Return(filepath.Join(rootDir, "osqueryd"))
			mockSack.On("OsqueryFlags").Return(tt.osqueryFlags)
			mockSack.On("Slogger").Return(multislogger.NewNopLogger())
			mockSack.On("RootDirectory").Maybe().Return("whatever_the_root_launcher_dir_is")

			proc, _, err := StartProcess(mockSack, rootDir)

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

	// Binary is already downloaded to osquerydCacheDir -- create a symlink at outputFile,
	// rather than copying the file, to maybe avoid https://github.com/golang/go/issues/22315
	outputFile := filepath.Join(dir, "osqueryd")
	sourceFile := filepath.Join(osquerydCacheDir, fmt.Sprintf("osqueryd-%s-stable", runtime.GOOS), "osqueryd")
	if err := os.Symlink(sourceFile, outputFile); err != nil {
		return fmt.Errorf("creating symlink from %s to %s: %w", sourceFile, outputFile, err)
	}

	return nil
}
