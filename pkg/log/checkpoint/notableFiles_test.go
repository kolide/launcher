package checkpoint

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNotableFilesFound(t *testing.T) {
	const dirsToCreate = 2
	const filesPerDir = 2
	tempDirs, filePaths := setupNotableFiles(dirsToCreate, filesPerDir)

	// update package var to search in temp dirs, make sure it finds files
	notableFileDirs[runtime.GOOS] = tempDirs

	foundFilesPaths := notableFilePaths()

	deleteDirs(tempDirs)

	require.Equal(t, filePaths, foundFilesPaths)
	require.Equal(t, dirsToCreate*filesPerDir, len(foundFilesPaths))
}

func TestNotableFilesNotFound(t *testing.T) {
	notableFileDirs[runtime.GOOS] = []string{"/i/dont/exist/hopefully", "/hopefully/i/dont/exists/either"}
	expectedOutput := []string{}
	for _, dir := range notableFileDirs[runtime.GOOS] {
		pathErr := fs.PathError{
			Op:   "open",
			Path: dir,
			// would expect this to be constant or var somewhere in os package, but couldn't find
			Err: errors.New("no such file or directory"),
		}
		expectedOutput = append(expectedOutput, pathErr.Error())
	}

	foundFilesPaths := notableFilePaths()
	require.Equal(t, expectedOutput, foundFilesPaths)
}

func TestNotableFileDirsNotDefinedForOS(t *testing.T) {
	delete(notableFileDirs, runtime.GOOS)
	expectedOutput := []string{notableFilesDirNotDefinedMsg(runtime.GOOS)}
	foundFilesPaths := notableFilePaths()
	require.Equal(t, expectedOutput, foundFilesPaths)
}

func setupNotableFiles(dirCount int, filesPerDir int) (tempDirs []string, filePaths []string) {
	// overwrite the varible that defines the dir path to check for files
	filePaths = []string{}

	for i := 0; i < dirCount; i++ {
		dir, err := os.MkdirTemp("", "notable-files-unit-test")
		if err != nil {
			panic(err)
		}
		tempDirs = append(tempDirs, dir)

		for j := 0; j < filesPerDir; j++ {

			filePath := dir + "/notableFile-" + fmt.Sprint(j) + ".txt"
			file, err := os.Create(filePath)
			if err != nil {
				panic(err)
			}
			file.Close()

			filePaths = append(filePaths, filePath)
		}
	}

	return tempDirs, filePaths
}

func deleteDirs(dirnames []string) {
	for _, dir := range dirnames {
		err := os.RemoveAll(dir)
		if err != nil {
			panic(err)
		}
	}
}
