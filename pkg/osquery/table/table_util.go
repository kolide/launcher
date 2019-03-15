package table

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
)

var homedirLocations = map[string][]string{
	"windows": {"/Users"}, // windows10 uses /Users
	"darwin":  {"/Users"},
	"default": {"/home"},
}

// findFileInUserDirs looks for the existence of a specified path as a
// subdirectory of users' home directories. It does this by searching
// likely paths
func findFileInUserDirs(path string, user string) ([]string, error) {
	var foundDirs []string

	homedirRoots, ok := homedirLocations[runtime.GOOS]
	if !ok {
		homedirRoots, ok = homedirLocations["default"]
		if !ok {
			return foundDirs, errors.New("No homedir locations for this platform")
		}
	}

	for _, userDir := range homedirRoots {

		userDirs, err := ioutil.ReadDir(userDir)
		if err != nil {
			continue
		}

		for _, dir := range userDirs {
			fullPath := filepath.Join(userDir, dir.Name(), path)
			if stat, err := os.Stat(fullPath); err == nil && stat.Mode().IsRegular() {
				foundDirs = append(foundDirs, fullPath)
			}
		}
	}

	return foundDirs, nil
}
