//go:build linux

package nativemessaging

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/shirou/gopsutil/v4/process"
)

// allowlistedBrowsers maps is a lookup of allowlisted browsers on Linux.
// In case of variable install locations, we allowlist the executable filename rather than
// the full path.
var allowlistedBrowsers = map[string]struct{}{
	"chrome":                   {},
	"chromium":                 {},
	"chromium-browser":         {},
	"chromium-browser-privacy": {},
}

// validateBrowser checks that the process is a known browser where the executable
// is owned by root with appropriate permissions.
func validateBrowser(ctx context.Context, proc *process.Process) error {
	pathToVerify, err := proc.ExeWithContext(ctx)
	if err != nil {
		return fmt.Errorf("getting executable for browser process: %w", err)
	}

	// Confirm that this is a known browser
	if _, found := allowlistedBrowsers[filepath.Base(pathToVerify)]; !found {
		return fmt.Errorf("filename of executable %s for browser process not in allowlisted browser names", pathToVerify)
	}

	// We can't check codesigning, so confirm that this path is root-owned
	// and not group- or world-writable.
	fi, err := os.Stat(pathToVerify)
	if err != nil {
		return fmt.Errorf("getting file info for %s: %w", pathToVerify, err)
	}
	fileStats, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("getting stat_t for %s", pathToVerify)
	}
	if fileStats.Uid != 0 {
		return fmt.Errorf("browser %s is not root-owned -- owned by %d", pathToVerify, fileStats.Uid)
	}
	if fi.Mode()&022 != 0 {
		return fmt.Errorf("%s is group- or world-writable: %s", pathToVerify, fi.Mode().String())
	}

	return nil
}
