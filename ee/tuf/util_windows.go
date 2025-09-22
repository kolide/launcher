//go:build windows
// +build windows

package tuf

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kolide/launcher/ee/observability"
)

// executableLocation returns the path to the executable in `updateDirectory`.
func executableLocation(updateDirectory string, binary autoupdatableBinary) string {
	return filepath.Join(updateDirectory, fmt.Sprintf("%s.exe", binary))
}

// checkExecutablePermissions checks whether a specific file looks
// like it's executable. This is used in evaluating whether something
// is an updated version.
//
// Windows does not have executable bits, so we omit those. And
// instead check the file extension.
func checkExecutablePermissions(ctx context.Context, potentialBinary string) error {
	_, span := observability.StartSpan(ctx)
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
	case !strings.HasSuffix(potentialBinary, ".exe"):
		return errors.New("not executable")
	}

	return nil
}
