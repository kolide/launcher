// Package augeas provides some lenses for use with osquery's augeas
// support.
//
// As we run a subset of the lenses, the simplest way to check for
// missing dependencies is to augtool:
//
//	augtool -c -S  -I ./assets/lenses
//
// Also handy is seeing what files this will ingest:
//
//	augtool  -S  -I .//assets/lenses print /files//*
//
// You can also explore the augeas space via osquery. Some interesting examples:
//
//	select * from augeas;
//	select * from augeas where node like "/augeas/load/%";     (requies osquery patches)
//	select * from augeas where node like "/augeas/files/%%";   (requies osquery patches)
package augeas

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed assets/*
var assets embed.FS

const lensSubdir = "assets/lenses"

func InstallLenses(targetDir string) error {
	lensesFS, err := fs.Sub(assets, lensSubdir)
	if err != nil {
		return fmt.Errorf("reading lenses subdirectory %s: %w", lensSubdir, err)
	}

	if err := fs.WalkDir(lensesFS, ".", genCopyLensToDiskFunc(lensesFS, targetDir)); err != nil {
		return fmt.Errorf("walking lenes directory: %w", err)
	}

	return nil
}

// genCopyLensToDiskFunc returns fs.WalkDirFunc function that will
// copy files to disk in a given location.
func genCopyLensToDiskFunc(srcFS fs.FS, targetdir string) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Should be impossible for embed https://github.com/golang/go/issues/45815
			return fmt.Errorf("Impossible error from embed: %w", err)
		}

		if path == lensSubdir || path == "." {
			return nil
		}

		if d.IsDir() {
			return fmt.Errorf("Lenses can only be files, %s is a directory", path)
		}

		data, err := fs.ReadFile(srcFS, path)
		if err != nil {
			// Should be impossible for embded https://github.com/golang/go/issues/45815
			return fmt.Errorf("Impossible error from embed: %w", err)
		}

		if err := os.WriteFile(filepath.Join(targetdir, path), data, 0644); err != nil {
			return fmt.Errorf("writing lens %s, got: %w", path, err)
		}

		return nil
	}
}
