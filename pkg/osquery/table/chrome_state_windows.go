package table

import (
	"io/ioutil"
	"os"
	"path/filepath"
)

const (
	usersDir = `C:\Users`
	userData = `AppData\Local\Google\Chrome\User Data\Local State`
)

func findChromeStateFiles() []string {
	chromeLocalStateFiles := []string{}
	filesInUser, err := ioutil.ReadDir(usersDir)
	if err != nil {
		return []string{}
	}

	for _, f := range filesInUser {
		if f.IsDir() && (f.Name() != "Public") {
			stateFilePath := filepath.Join(usersDir, f.Name(), userData)
			if _, err := os.Stat(stateFilePath); err == nil {
				chromeLocalStateFiles = append(chromeLocalStateFiles, stateFilePath)
			}
		}
	}

	return chromeLocalStateFiles
}
