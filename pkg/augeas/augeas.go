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

	"github.com/kolide/kit/fsutil"
)

//go:embed assets/*
var assets embed.FS

const lensSubdir = "assets/lenses"

func InstallLenses(targetDir string) error {

	lensesFS, err := fs.Sub(assets, lensSubdir)
	if err != nil {
		return fmt.Errorf("reading lenses subdirectory %s: %w", lensSubdir, err)
	}

	return fsutil.CopyFSToDisk(lensesFS, targetDir, fsutil.CommonFileMode)
}
