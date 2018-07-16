package osqtable

import (
	"io/ioutil"
	"os"
	"path/filepath"
)

const (
	usersDir = "/home"
	userData = ".config/google-chrome/Local State"
)

func findChromeStateFiles() []string {
	chromeLocalStateFiles := []string{}
	filesInUser, err := ioutil.ReadDir(usersDir)
	if err != nil {
		return []string{}
	}

	for _, f := range filesInUser {
		stateFilePath := filepath.Join(usersDir, f.Name(), userData)
		if _, err := os.Stat(stateFilePath); err == nil {
			chromeLocalStateFiles = append(chromeLocalStateFiles, stateFilePath)
		}
	}

	return chromeLocalStateFiles
}
