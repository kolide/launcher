package interactive

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kolide/kit/fs"
	"github.com/kolide/launcher/pkg/packaging"
	"github.com/pkg/errors"
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
			name:         "no flags",
			osqueryFlags: []string{},
			wantProc:     true,
		},
		{
			name: "extension socket flag",
			osqueryFlags: []string{
				fmt.Sprintf("extensions_socket=%s", filepath.Join(t.TempDir(), "test.sock")),
			},
			wantProc: true,
		},
		{
			name: "no socket val",
			osqueryFlags: []string{
				"extensions_socket=",
			},
			wantProc:       false,
			errContainsStr: "extensions_socket flag is missing a value",
		},
		{
			name: "socket path too long",
			osqueryFlags: []string{
				fmt.Sprintf("extensions_socket=%s", filepath.Join(t.TempDir(), "this_is_a_really_really_long_socket")),
			},
			wantProc:       false,
			errContainsStr: "socket path is too long",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rootDir := t.TempDir()
			require.NoError(t, downloadOsquery(rootDir))
			osquerydPath := filepath.Join(rootDir, "osqueryd")

			proc, err := StartProcess(rootDir, osquerydPath, tt.osqueryFlags)

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
		return errors.Wrapf(err, "Error parsing platform: %s", runtime.GOOS)
	}

	outputFile := filepath.Join(dir, "osqueryd")
	cacheDir := os.TempDir()

	path, err := packaging.FetchBinary(context.TODO(), cacheDir, "osqueryd", target.PlatformBinaryName("osqueryd"), "stable", target)
	if err != nil {
		return errors.Wrap(err, "An error occurred fetching the osqueryd binary")
	}

	if err := fs.CopyFile(path, outputFile); err != nil {
		return errors.Wrapf(err, "Couldn't copy file to %s", outputFile)
	}

	return nil
}

// findOsquery will attempt to find osquery. We don't much care about
// errors here, either we find it, or we don't.
func findOsquery() string {
	osqBinaryName := "osqueryd"
	if runtime.GOOS == "windows" {
		osqBinaryName = osqBinaryName + ".exe"
	}

	var likelyDirectories []string

	if exPath, err := os.Executable(); err == nil {
		likelyDirectories = append(likelyDirectories, filepath.Dir(exPath))
	}

	// Places to check. We could conditionalize on GOOS, but it doesn't
	// seem important.
	likelyDirectories = append(
		likelyDirectories,
		"/usr/local/kolide/bin",
		"/usr/local/kolide-k2/bin",
		"/usr/local/bin",
		`C:\Program Files\osquery`,
	)

	for _, dir := range likelyDirectories {
		maybeOsq := filepath.Join(filepath.Clean(dir), osqBinaryName)

		info, err := os.Stat(maybeOsq)
		if err != nil {
			continue
		}

		if info.IsDir() {
			continue
		}

		// I guess it's good enough...
		return maybeOsq
	}

	// last ditch, check for osquery on the PATH
	if osqPath, err := exec.LookPath(osqBinaryName); err == nil {
		return osqPath
	}

	return ""
}
