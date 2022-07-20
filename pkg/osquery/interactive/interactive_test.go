//go:build !windows
// +build !windows

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

func TestStartProcess(t *testing.T) {
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
			name:           "make socket path to long to test our error handling, making this really long causes the socket path to be long",
			wantProc:       false,
			errContainsStr: "exceeded the maximum socket path character length",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

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
	cacheDir := filepath.Join(os.TempDir(), "launcher_interactive_tests")

	if err := os.MkdirAll(cacheDir, fs.DirMode); err != nil {
		return fmt.Errorf("error creating cache dir: %w", err)
	}

	path, err := packaging.FetchBinary(context.TODO(), cacheDir, "osqueryd", target.PlatformBinaryName("osqueryd"), "stable", target)
	if err != nil {
		return fmt.Errorf("error fetching binary osqueryd binary: %w", err)
	}

	if err := fs.CopyFile(path, outputFile); err != nil {
		return fmt.Errorf("error copying binary osqueryd binary: %w", err)
	}

	return nil
}
