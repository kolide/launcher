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
	ctx, cancel := context.WithCancel(t.Context())
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
		in       string
		platform PlatformFlavor
		out      string
	}{
		{
			in:       "0.9.2-26-g6146437",
			platform: Windows,
			out:      "0.9.2.26",
		},
		{
			in:       "0.9.3-44",
			platform: Windows,
			out:      "0.9.3.44",
		},

		{
			in:       "0.9.5",
			platform: Windows,
			out:      "0.9.5.0",
		},
		{
			in:       "0.9.2-26-g6146437",
			platform: Darwin,
			out:      "0.9.2",
		},
		{
			in:       "0.9.3-44",
			platform: Darwin,
			out:      "0.9.3",
		},
		{
			in:       "v0.9.5",
			platform: Darwin,
			out:      "0.9.5",
		},
		{
			in:       "v10.8.2-1002df",
			platform: Darwin,
			out:      "10.8.2",
		},
		{
			in:       "0.9.2-26-g6146437",
			platform: Linux,
			out:      "0.9.2-26-g6146437",
		},
	}

	for _, tt := range tests {
		version, err := formatVersion(tt.in, tt.platform)
		require.NoError(t, err)
		require.Equal(t, tt.out, version)
	}
}

func TestLauncherLocation_Darwin(t *testing.T) {
	t.Parallel()

	pDarwin := &PackageOptions{
		target: Target{
			Platform: Darwin,
		},
		packageRoot: t.TempDir(),
		binDir:      "bin",
	}

	// First, test that if the app bundle doesn't exist, we fall back to the top-level binary
	expectedFallbackLauncherLocation := filepath.Join(pDarwin.packageRoot, pDarwin.binDir, "launcher")
	require.Equal(t, expectedFallbackLauncherLocation, pDarwin.launcherLocation())

	// Create a temp directory with an app bundle in it
	binDir := filepath.Join(pDarwin.packageRoot, pDarwin.binDir)
	require.NoError(t, os.MkdirAll(binDir, 0755))
	baseDir := filepath.Join(pDarwin.packageRoot, "Kolide.app", "Contents", "MacOS")
	require.NoError(t, os.MkdirAll(baseDir, 0755))
	expectedLauncherBinaryPath := filepath.Join(baseDir, "launcher")
	f, err := os.Create(expectedLauncherBinaryPath)
	require.NoError(t, err, "could not create temp file for test")
	defer f.Close()
	defer os.Remove(expectedLauncherBinaryPath)

	// Now, confirm that we find the binary inside the app bundle
	require.Equal(t, expectedLauncherBinaryPath, pDarwin.launcherLocation())
}

func TestLauncherLocation_Windows(t *testing.T) {
	t.Parallel()

	pWindows := &PackageOptions{
		target: Target{
			Platform: Windows,
			Arch:     Amd64,
		},
		packageRoot: t.TempDir(),
		binDir:      "bin",
	}

	// No file check for windows, just expect the binary in the given location
	expectedLauncherLocation := filepath.Join(pWindows.packageRoot, pWindows.binDir, string(pWindows.target.Arch), "launcher.exe")
	require.Equal(t, expectedLauncherLocation, pWindows.launcherLocation())
}

func TestLauncherLocation_Linux(t *testing.T) {
	t.Parallel()

	pLinux := &PackageOptions{
		target: Target{
			Platform: Linux,
		},
		packageRoot: t.TempDir(),
		binDir:      "bin",
	}

	// No file check for Linux, just expect the binary in the given location
	expectedLauncherLocation := filepath.Join(pLinux.packageRoot, pLinux.binDir, "launcher")
	require.Equal(t, expectedLauncherLocation, pLinux.launcherLocation())
}
