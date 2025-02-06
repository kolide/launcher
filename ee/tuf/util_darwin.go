//go:build darwin
// +build darwin

package tuf

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kolide/launcher/pkg/traces"
)

// executableLocation returns the path to the executable in `updateDirectory`.
// For launcher, and versions of osquery after 5.9.1, this means a path inside the app bundle.
func executableLocation(updateDirectory string, binary autoupdatableBinary) string {
	switch binary {
	case binaryLauncher:
		return filepath.Join(updateDirectory, "Kolide.app", "Contents", "MacOS", string(binary))
	case binaryOsqueryd:
		// Only return the path to the app bundle executable if it exists
		appBundleExecutable := filepath.Join(updateDirectory, "osquery.app", "Contents", "MacOS", string(binary))
		if _, err := os.Stat(appBundleExecutable); err == nil {
			return appBundleExecutable
		}

		// Older version of osquery
		return filepath.Join(updateDirectory, string(binary))
	default:
		return ""
	}
}

// checkExecutablePermissions checks whether a specific file looks
// like it's executable. This is used in evaluating whether something
// is an updated version.
func checkExecutablePermissions(ctx context.Context, potentialBinary string) error {
	_, span := traces.StartSpan(ctx)
	defer span.End()

	if potentialBinary == "" {
		return errors.New("empty string isn't executable")
	}
	stat, err := os.Stat(potentialBinary)
	switch {
	case os.IsNotExist(err):
		return errors.New("no such file")
	case err != nil:
		return fmt.Errorf("statting file: %w", err)
	case stat.IsDir():
		return errors.New("is a directory")
	case stat.Mode()&0111 == 0:
		return errors.New("not executable")
	}

	return nil
}
