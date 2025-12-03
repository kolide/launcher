package testutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/kolide/kit/fsutil"
	"github.com/kolide/launcher/pkg/packaging"
)

// DownloadOsquery downloads an osquery binary for testing purposes. It reuses existing
// binaries across test runs by storing them in a predictable location in the temp directory.
// The binary path includes the version, so new versions will be downloaded automatically.
//
// Parameters:
//   - version: The osquery version to download (e.g., "nightly", "stable", "5.17.0")
//
// Returns:
//   - The path to the osquery binary
//   - A cleanup function that can be called to remove the binary (optional)
//   - An error if the download fails
//
// The cleanup function is provided for tests that want to clean up after themselves,
// but you can also skip calling it to allow the binary to be reused across test runs.
func DownloadOsquery(version string) (binaryPath string, cleanup func() error, err error) {
	target := packaging.Target{}
	if err := target.PlatformFromString(runtime.GOOS); err != nil {
		return "", nil, fmt.Errorf("parsing platform %s: %w", runtime.GOOS, err)
	}
	target.Arch = packaging.ArchFlavor(runtime.GOARCH)
	if runtime.GOOS == "darwin" {
		target.Arch = packaging.Universal
	}

	// Create a predictable directory in temp for storing test osquery binaries
	// This allows reuse across test runs
	cacheDir := filepath.Join(os.TempDir(), "launcher-test-osquery")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", nil, fmt.Errorf("creating cache directory: %w", err)
	}

	// Create a filename that includes version and platform info
	// This ensures we download a new binary when the version changes
	binaryName := fmt.Sprintf("osqueryd-%s-%s-%s", version, runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath = filepath.Join(cacheDir, binaryName)

	// Download the binary if it doesn't exist
	if _, err := os.Stat(binaryPath); err != nil {
		// Binary doesn't exist, download it
		// Create a separate temp directory for the download (FetchBinary creates subdirectories)
		downloadDir, err := os.MkdirTemp("", "osquery-download-")
		if err != nil {
			return "", nil, fmt.Errorf("creating download directory: %w", err)
		}
		defer os.RemoveAll(downloadDir)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		dlPath, err := packaging.FetchBinary(ctx, downloadDir, "osqueryd", target.PlatformBinaryName("osqueryd"), version, target)
		if err != nil {
			return "", nil, fmt.Errorf("fetching osqueryd binary: %w", err)
		}

		// Copy to our standardized cache location
		if err := fsutil.CopyFile(dlPath, binaryPath); err != nil {
			return "", nil, fmt.Errorf("copying osqueryd binary from %s to %s: %w", dlPath, binaryPath, err)
		}
	}

	// Always ensure the binary is executable and has no quarantine attributes,
	// even when reusing a cached binary (these attributes might get re-applied)
	if err := os.Chmod(binaryPath, 0755); err != nil {
		return "", nil, fmt.Errorf("setting executable permissions: %w", err)
	}

	cleanupFunc := func() error {
		return os.Remove(binaryPath)
	}

	return binaryPath, cleanupFunc, nil
}
