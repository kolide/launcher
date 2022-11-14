package packaging

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Various helpers are in packaging_test.go

func TestLauncherVersionDetection(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var err error

	p := &PackageOptions{}
	p.execCC = helperCommandContext

	err = p.detectLauncherVersion(ctx)
	require.NoError(t, err)

	require.Equal(t, "0.5.6-19-g17c8589", p.PackageVersion)
}

func TestFormatVersion(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		in  string
		out string
	}{
		{
			in:  "0.9.2-26-g6146437",
			out: "0.9.2.26",
		},
		{
			in:  "0.9.3-44",
			out: "0.9.3.44",
		},

		{
			in:  "0.9.5",
			out: "0.9.5.0",
		},
	}

	for _, tt := range tests {
		version, err := formatVersion(tt.in)
		require.NoError(t, err)
		require.Equal(t, tt.out, version)
	}
}

func TestLauncherLocation(t *testing.T) {
	t.Parallel()

	pDarwin := &PackageOptions{target: Target{Platform: Darwin}}

	// First, test that if the app bundle doesn't exist, we fall back to the top-level binary
	require.Equal(t, "/some/dir/launcher", pDarwin.launcherLocation("/some/dir"))

	// Create a temp directory with an app bundle in it
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "Kolide.app", "Contents", "MacOS"), 0755)
	baseDir := filepath.Join(tmpDir, "Kolide.app/Contents/MacOS")
	expectedLauncherBinaryPath := filepath.Join(baseDir, "launcher")
	f, err := os.Create(expectedLauncherBinaryPath)
	if err != nil {
		t.Errorf("could not create temp file for test: %v", err)
		t.FailNow()
	}
	defer f.Close()
	defer os.Remove(expectedLauncherBinaryPath)

	// Now, confirm that we find the binary inside the app bundle
	require.Equal(t, expectedLauncherBinaryPath, pDarwin.launcherLocation(baseDir))

	// No file check for windows, just expect the binary in the given location
	pWindows := &PackageOptions{target: Target{Platform: Windows}}
	require.Equal(t, "/some/dir/launcher.exe", pWindows.launcherLocation("/some/dir"))

	// Same as for windows: no file check, just expect the binary in the given location
	pLinux := &PackageOptions{target: Target{Platform: Linux}}
	require.Equal(t, "/some/dir/launcher", pLinux.launcherLocation("/some/dir"))
}
