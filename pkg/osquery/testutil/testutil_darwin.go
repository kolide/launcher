//go:build darwin
// +build darwin

package testutil

import (
	"context"
	"fmt"
	"os"

	"github.com/kolide/launcher/ee/allowedcmd"
)

// prepareBinaryForExecution removes extended attributes and ad-hoc signs a binary on macOS
// to allow it to execute without Gatekeeper interference.
func prepareBinaryForExecution(binaryPath string) error {
	// Verify the binary exists before trying to sign it
	if _, err := os.Stat(binaryPath); err != nil {
		return fmt.Errorf("binary does not exist at %s: %w", binaryPath, err)
	}

	// Clear extended attributes (best effort)
	_ = removeAllExtendedAttributes(binaryPath)

	// Ad-hoc sign the binary to bypass Gatekeeper restrictions
	// This is required because com.apple.provenance cannot be removed with xattr
	signCmd, err := allowedcmd.Codesign(context.Background(), "--force", "--sign", "-", binaryPath)
	if err != nil {
		return fmt.Errorf("failed to create codesign command: %w", err)
	}
	if err := signCmd.Run(); err != nil {
		return fmt.Errorf("failed to ad-hoc sign binary at %s (required for macOS Gatekeeper): %w", binaryPath, err)
	}

	return nil
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
