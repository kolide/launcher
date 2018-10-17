package table

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

const userDir = "/Users"

// findFileInUserDirs looks for the existence of a specified path as a
// subdirectory of users' home directories on macOS
func findFileInUserDirs(path string) ([]string, error) {
	userDirs, err := ioutil.ReadDir(userDir)
	if err != nil {
		return nil, errors.Wrap(err, "Reading /Users/")
	}
	var res []string
	for _, dir := range userDirs {
		fullPath := filepath.Join(userDir, dir.Name(), path)
		if stat, err := os.Stat(fullPath); err == nil && stat.Mode().IsRegular() {
			res = append(res, fullPath)
		}
	}
	return res, nil
}
