package table

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
)

type findFile struct {
	username string
}

type FindFileOpt func(*findFile)

func WithUsername(username string) FindFileOpt {
	return func(ff *findFile) {
		ff.username = username
	}
}

var homedirLocations = map[string][]string{
	"windows": {"/Users"}, // windows10 uses /Users
	"darwin":  {"/Users"},
	"default": {"/home"},
}

// findFileInUserDirs looks for the existence of a specified path as a
// subdirectory of users' home directories. It does this by searching
// likely paths
func findFileInUserDirs(path string, opts ...FindFileOpt) ([]string, error) {
	ff := &findFile{}

	for _, opt := range opts {
		opt(ff)
	}

	var foundDirs []string

	homedirRoots, ok := homedirLocations[runtime.GOOS]
	if !ok {
		homedirRoots, ok = homedirLocations["default"]
		if !ok {
			return foundDirs, errors.New("No homedir locations for this platform")
		}
	}

	foundPaths := []string{}

	// Redo/remove when we make username a required parameter
	if ff.username == "" {
		for _, possibleHome := range homedirRoots {

			userDirs, err := ioutil.ReadDir(possibleHome)
			if err != nil {
				// This possibleHome doesn't exist. Move on
				continue
			}

			// For each user's dir, in this possibleHome, check!
			for _, ud := range userDirs {
				fullPath := filepath.Join(possibleHome, ud.Name(), path)
				if stat, err := os.Stat(fullPath); err == nil && stat.Mode().IsRegular() {
					foundPaths = append(foundPaths, fullPath)
				}
			}
		}

		return foundPaths, nil
	}

	// We have a username. Future normal path here
	for _, possibleHome := range homedirRoots {
		fullPath := filepath.Join(possibleHome, ff.username, path)
		if stat, err := os.Stat(fullPath); err == nil && stat.Mode().IsRegular() {
			foundPaths = append(foundPaths, fullPath)
		}
	}
	return foundPaths, nil

}
