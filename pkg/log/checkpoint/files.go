package checkpoint

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

func fileNamesInDirs(dirnames ...string) map[string]string {
	results := make(map[string]string)

	for _, dirname := range dirnames {
		files, err := os.ReadDir(dirname)

		switch {
		case errors.Is(err, os.ErrNotExist):
			results[dirname] = "not present"
		case err != nil:
			results[dirname] = err.Error()
		case len(files) == 0:
			results[dirname] = "present, but empty"
		default:
			fileToLog := make([]string, len(files))

			for i, file := range files {
				fileToLog[i] = file.Name()
			}

			results[dirname] = fmt.Sprintf("contains: %s", strings.Join(fileToLog, ", "))
		}
	}

	return results
}
