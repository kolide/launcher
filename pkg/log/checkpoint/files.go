package checkpoint

import (
	"fmt"
	"os"
	"path/filepath"
)

func fileNamesInDirs(dirnames ...string) []string {
	results := []string{}

	for _, dirname := range dirnames {
		files, err := os.ReadDir(dirname)

		switch {
		case err != nil:
			results = append(results, err.Error())
		case len(files) == 0:
			results = append(results, emptyDirMsg(dirname))
		default:
			for _, file := range files {
				results = append(results, filepath.Join(dirname, file.Name()))
			}
		}
	}

	return results
}

// emptyDirMsg is a helper method to generate empty dir message, makes testing easier
func emptyDirMsg(dirname string) string {
	return fmt.Sprintf("%s is an empty directory", dirname)
}
