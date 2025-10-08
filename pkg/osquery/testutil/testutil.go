package testutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/kolide/kit/fsutil"
	"github.com/kolide/launcher/ee/allowedcmd"
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
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		dlPath, err := packaging.FetchBinary(ctx, cacheDir, "osqueryd", target.PlatformBinaryName("osqueryd"), version, target)
		if err != nil {
			return "", nil, fmt.Errorf("fetching osqueryd binary: %w", err)
		}

		// Copy to our standardized location
		if err := fsutil.CopyFile(dlPath, binaryPath); err != nil {
			return "", nil, fmt.Errorf("copying osqueryd binary: %w", err)
		}

		// Clean up the download path (FetchBinary downloads to a temp location)
		os.Remove(dlPath)
	}

	// Always ensure the binary is executable and has no quarantine attributes,
	// even when reusing a cached binary (these attributes might get re-applied)
	if runtime.GOOS != "windows" {
		if err := os.Chmod(binaryPath, 0755); err != nil {
			return "", nil, fmt.Errorf("setting executable permissions: %w", err)
		}
	}

	// Remove ALL extended attributes on macOS that might prevent execution
	// and ad-hoc sign the binary to bypass Gatekeeper restrictions
	if runtime.GOOS == "darwin" {
		// Clear extended attributes (best effort)
		_ = removeAllExtendedAttributes(binaryPath)

		// Ad-hoc sign the binary to bypass Gatekeeper restrictions
		// This is required because com.apple.provenance cannot be removed with xattr
		signCmd, err := allowedcmd.Codesign(context.Background(), "--force", "--sign", "-", binaryPath)
		if err != nil {
			return "", nil, fmt.Errorf("failed to create codesign command: %w", err)
		}
		if err := signCmd.Run(); err != nil {
			// Signing failed - binary may not execute, but continue anyway
			return "", nil, fmt.Errorf("failed to ad-hoc sign binary (required for macOS Gatekeeper): %w", err)
		}
	}

	cleanupFunc := func() error {
		return os.Remove(binaryPath)
	}

	return binaryPath, cleanupFunc, nil
}

// removeAllExtendedAttributes removes ALL extended attributes from a file on macOS
// This is a best-effort operation - some attributes like com.apple.provenance may remain
func removeAllExtendedAttributes(path string) error {
	// Use xattr -c to clear ALL extended attributes
	cmd, err := allowedcmd.Xattr(context.Background(), "-c", path)
	if err != nil {
		return nil // Ignore errors - command might not be available
	}
	_ = cmd.Run() // Ignore errors - attributes might not exist
	return nil
}

// SignBinary ad-hoc signs a binary on macOS to allow it to execute.
// This must be called after copying a binary to a new location, as copying invalidates signatures.
// Note: Currently unused since we use symlinks instead of copies, but kept for potential future use.
func SignBinary(binaryPath string) error {
	if runtime.GOOS != "darwin" {
		return nil
	}

	// Remove extended attributes first
	_ = removeAllExtendedAttributes(binaryPath)

	// Ensure executable permissions
	if err := os.Chmod(binaryPath, 0755); err != nil {
		return fmt.Errorf("setting executable permissions: %w", err)
	}

	// Ad-hoc sign the binary
	signCmd, err := allowedcmd.Codesign(context.Background(), "--force", "--sign", "-", binaryPath)
	if err != nil {
		return fmt.Errorf("failed to create codesign command: %w", err)
	}
	if err := signCmd.Run(); err != nil {
		return fmt.Errorf("failed to ad-hoc sign binary: %w", err)
	}

	return nil
}

// DownloadOsqueryOrDie downloads an osquery binary for testing purposes, calling os.Exit(1)
// if the download fails. This is useful in TestMain functions where error handling is awkward.
//
// Parameters:
//   - version: The osquery version to download (e.g., "nightly", "stable", "5.17.0")
//
// Returns:
//   - The path to the osquery binary
//   - A cleanup function that can be called to remove the binary (optional)
func DownloadOsqueryOrDie(version string) (binaryPath string, cleanup func() error) {
	binaryPath, cleanup, err := DownloadOsquery(version)
	if err != nil {
		fmt.Printf("failed to download osqueryd binary for tests: %v\n", err)
		os.Exit(1) //nolint:forbidigo // Fine to use os.Exit inside tests
	}
	return binaryPath, cleanup
}
