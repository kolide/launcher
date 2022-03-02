package checkpoint

import (
	"os"
)

func fileNamesInDirs(dirnames ...string) []string {
	results := []string{}

	for _, dirname := range dirnames {
		files, err := os.ReadDir(dirname)

		if err != nil {
			results = append(results, err.Error())
			continue
		}

		for _, file := range files {
			results = append(results, dirname+"/"+file.Name())
		}
	}

	return results
}
