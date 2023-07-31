package checkups

import (
	"fmt"
	"io"
	"io/fs"
	"path/filepath"

	"golang.org/x/exp/maps"
)

// recursiveDirectoryContents descends through a directory, writing the files found to the provided `extraFH`.
// It returns the count of files at the top level, and any errors. This can be used to answer checkup
// questions like does this dir have at least 3 files, and list the full contents as extra.
func recursiveDirectoryContents(extraFH io.Writer, basedir string) (int, error) {
	// I'm not sure why, but WalkDir seems to visit some things twice. Meanwhile, just use a map to to count things
	files := make(map[string]bool, 0)

	filewalkErr := filepath.WalkDir(basedir, func(path string, d fs.DirEntry, err error) error {
		if filepath.Dir(path) == basedir {
			files[path] = true
		}

		fmt.Fprintln(extraFH, path)
		return nil
	})

	return len(maps.Keys(files)), filewalkErr

}
