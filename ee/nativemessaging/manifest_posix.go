//go:build !windows

package nativemessaging

import (
	"fmt"
	"os"
	"path/filepath"
)

// registerManifestFileLocation creates a symlink at `registrationPath` (the well-known path
// for Chrome) to `manifestFileLocation`, the file we've written to the launcher root directory.
func registerManifestFileLocation(manifestFileLocation string, registrationPath string) error {
	// Check if the symlink already exists and points to manifestFileLocation --
	// if it does, no need to create it.
	if _, err := os.Lstat(registrationPath); err == nil {
		resolvedRegistrationPath, resolvedRegistrationPathErr := filepath.EvalSymlinks(registrationPath)

		// We have to eval the manifestFileLocation too, to resolve `/var` to `/private/var` on macOS
		resolvedManifestFileLocation, resolvedManifestFileLocationErr := filepath.EvalSymlinks(manifestFileLocation)

		if resolvedRegistrationPathErr == nil && resolvedManifestFileLocationErr == nil && resolvedManifestFileLocation == resolvedRegistrationPath {
			return nil
		}

		// registrationPath exists, but it's not pointing to manifestFileLocation -- remove it
		// so we can set it correctly below.
		if err := os.Remove(registrationPath); err != nil {
			return fmt.Errorf("removing old symlink at %s before re-creating: %w", registrationPath, err)
		}
	}

	// We may need to create this directory before writing to it.
	registrationDir := filepath.Dir(registrationPath)
	if err := os.MkdirAll(registrationDir, 0755); err != nil {
		return fmt.Errorf("creating manifest registration directory %s: %w", registrationDir, err)
	}

	if err := os.Symlink(manifestFileLocation, registrationPath); err != nil {
		return fmt.Errorf("creating symlink at %s to %s: %w", registrationPath, manifestFileLocation, err)
	}

	return nil
}

func deregisterManifestFileLocation(registrationPath string) error {
	if err := os.Remove(registrationPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing symlink at %s: %w", registrationPath, err)
	}

	return nil
}
