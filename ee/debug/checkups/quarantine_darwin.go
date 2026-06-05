//go:build darwin

package checkups

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kolide/launcher/v2/ee/allowedcmd"
	"github.com/kolide/launcher/v2/pkg/launcher"
)

// quarantinedAppBundles finds all Kolide app bundle locations on disk and checks each
// to see if the quarantine xattr is set.
func (q *quarantine) quarantinedAppBundles(ctx context.Context) ([]string, error) {
	// Get the original launcher/osquery install locations
	binDir := launcher.DefaultPath(launcher.BinDirectory)
	if q.k.Identifier() != launcher.DefaultLauncherIdentifier {
		binDir = strings.ReplaceAll(binDir, launcher.DefaultLauncherIdentifier, q.k.Identifier())
	}
	pathsToCheck := []string{
		filepath.Join(binDir, "Kolide.app"),
		filepath.Join(binDir, "osquery.app"),
	}

	// Add any app bundles from the update directory
	var cumulativeErrors error
	launcherUpdates, err := filepath.Glob(filepath.Join(q.k.RootDirectory(), "updates", "launcher", "*", "Kolide.app"))
	if err != nil {
		cumulativeErrors = errors.Join(cumulativeErrors, fmt.Errorf("globbing for launcher updates: %w", err))
	} else if len(launcherUpdates) > 0 {
		pathsToCheck = append(pathsToCheck, launcherUpdates...)
	}
	osqueryUpdates, err := filepath.Glob(filepath.Join(q.k.RootDirectory(), "updates", "osqueryd", "*", "osquery.app"))
	if err != nil {
		cumulativeErrors = errors.Join(cumulativeErrors, fmt.Errorf("globbing for osqueryd updates: %w", err))
	} else if len(osqueryUpdates) > 0 {
		pathsToCheck = append(pathsToCheck, osqueryUpdates...)
	}

	// Check each file to see if it has com.apple.quarantine xattr set on it
	bundlesWithQuarantineXattrSet := make([]string, 0)
	for _, bundlePathToCheck := range pathsToCheck {
		isQuarantined, err := quarantineXattrSet(ctx, bundlePathToCheck)
		if err != nil {
			cumulativeErrors = errors.Join(cumulativeErrors, fmt.Errorf("checking quarantine xattr for %s: %w", bundlePathToCheck, err))
		}
		if isQuarantined {
			bundlesWithQuarantineXattrSet = append(bundlesWithQuarantineXattrSet, bundlePathToCheck)
		}
	}

	return bundlesWithQuarantineXattrSet, cumulativeErrors
}

func quarantineXattrSet(ctx context.Context, fileToCheck string) (bool, error) {
	cmd, err := allowedcmd.Xattr.Cmd(ctx, "-p", "com.apple.quarantine", fileToCheck)
	if err != nil {
		return false, fmt.Errorf("creating xattr command: %w", err)
	}

	out, err := cmd.CombinedOutput()
	if err != nil || strings.Contains(string(out), "No such xattr") {
		return false, nil
	}

	return true, nil
}
