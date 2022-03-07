package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kolide/kit/version"
)

const (
	versionFileFormat = "launcher-version-%s"
)

// RecordLauncherVersion stores the launcher version in the filesystem
// somewhere osquery can find it. This is to facilitate including it
// in the initial enrollment information that Kolide K2 uses. Because
// osquery does not have broad file read abilities, we instead
// leverage a filename.
//
// Both launcher, and osquery, are based around there only being a
// single instance of them running in a given root directory. As such,
// this writes a single file, cleans up any old ones.
func RecordLauncherVersion(rootDir string) error {
	verFile := makeFilePath(rootDir, version.Version().Version)

	existingFiles, err := filepath.Glob(makeFilePath(rootDir, "*"))
	if err != nil {
		// errors here indicate a bad glob, possible indicative of a bad rootDir
		return fmt.Errorf("expanding glob, maybe a bad dir: %w", err)
	}

	// simplest case. There's a file, and it matches.
	if len(existingFiles) == 1 && existingFiles[0] == verFile {
		return nil
	}

	// no files
	if len(existingFiles) == 0 {
		return touchFile(verFile)
	}

	// More complicated cases
	foundFile := false
	for _, f := range existingFiles {
		if f == verFile {
			foundFile = true
			continue
		}

		if err := os.Remove(f); err != nil {
			return fmt.Errorf("removing old version file %s, got: %w", f, err)
		}
	}

	if !foundFile {
		return touchFile(verFile)
	}

	return nil
}

func touchFile(filename string) error {
	return os.WriteFile(filename, []byte(time.Now().String()), 0644)
}

func makeFilePath(rootDir, ver string) string {
	return filepath.Join(rootDir, fmt.Sprintf(versionFileFormat, ver))
}
