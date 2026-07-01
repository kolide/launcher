//go:build linux

package nativemessaging

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// allowlistedBrowsers maps is a lookup of allowlisted browsers on Linux.
// In case of variable install locations, we allowlist the executable filename rather than
// the full path.
var allowlistedBrowsers = map[string]struct{}{
	"chrome":                   {},
	"chromium":                 {},
	"chromium-browser":         {},
	"chromium-browser-privacy": {}, // RPM
	"xdg-desktop-portal":       {}, // Firefox distributed via Snap or Flatpak
}

// validateBrowser checks that the given path is a known browser where the executable
// is owned by root with appropriate permissions.
func validateBrowser(_ context.Context, browserPath string, _ string) error {
	// Confirm that this is a known browser
	if _, found := allowlistedBrowsers[filepath.Base(browserPath)]; !found {
		return fmt.Errorf("filename of executable %s for browser process not in allowlisted browser names", browserPath)
	}

	// We can't check codesigning, so confirm that this path is root-owned
	// and not group- or world-writable.
	fi, err := os.Stat(browserPath)
	if err != nil {
		return fmt.Errorf("getting file info for %s: %w", browserPath, err)
	}
	fileStats, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("getting stat_t for %s", browserPath)
	}
	if fileStats.Uid != 0 {
		return fmt.Errorf("browser %s is not root-owned -- owned by %d", browserPath, fileStats.Uid)
	}
	if fi.Mode()&022 != 0 {
		return fmt.Errorf("%s is group- or world-writable: %s", browserPath, fi.Mode().String())
	}

	return nil
}
