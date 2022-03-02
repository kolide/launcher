package checkpoint

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"sort"
	"testing"

	"github.com/kolide/kit/ulid"
	"github.com/stretchr/testify/suite"
)

type FileSuite struct {
	suite.Suite
	tempDir string
}

func (s *FileSuite) SetupSuite() {
	dir, err := os.MkdirTemp("", "notable-files-unit-test")
	s.Require().NoError(err, "making temp dir")
	s.tempDir = dir
}

func (s *FileSuite) TearDownSuite() {
	s.Require().NoError(os.RemoveAll(s.tempDir), "deleting temp dir")
}

func (s *FileSuite) TestNotableFilesFound() {
	var tests = []struct {
		dirsToCreate int
		filesPerDir  int
	}{
		{dirsToCreate: 2, filesPerDir: 2},
		{dirsToCreate: 2, filesPerDir: 0},
		{dirsToCreate: 0, filesPerDir: 0},
	}

	for _, tt := range tests {
		dirs, expectedPaths, err := createTestFiles(s.tempDir, tt.dirsToCreate, tt.filesPerDir)
		s.NoError(err, "creating test files")

		foundPaths := fileNamesInDirs(dirs...)
		sort.Strings(foundPaths)
		s.Require().Equal(expectedPaths, foundPaths)
		s.Require().Equal(tt.dirsToCreate*tt.filesPerDir, len(foundPaths))
	}
}

func (s *FileSuite) TestDirNotFound() {
	const pathFmt = "%s/%s"
	nonExistantDirs := []string{
		fmt.Sprintf(pathFmt, s.tempDir, ulid.New()),
		fmt.Sprintf(pathFmt, s.tempDir, ulid.New()),
	}

	expectedOutput := []string{}

	for _, dir := range nonExistantDirs {
		pathErr := fs.PathError{
			Op:   "open",
			Path: dir,
			// would expect this to be constant or var somewhere in os package, but couldn't find
			Err: errors.New("no such file or directory"),
		}

		// not found error is different for windows
		if runtime.GOOS == "windows" {
			pathErr.Err = errors.New("The system cannot find the path specified.")
		}

		expectedOutput = append(expectedOutput, pathErr.Error())
	}

	foundPaths := fileNamesInDirs(nonExistantDirs...)

	s.Require().Equal(expectedOutput, foundPaths)
	s.Require().Equal(len(nonExistantDirs), len(foundPaths))
}

func TestFileSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(FileSuite))
}

func createTestFiles(baseDir string, dirCount int, filesPerDir int) (dirs []string, files []string, err error) {
	files = []string{}

	for i := 0; i < dirCount; i++ {
		dir, err := os.MkdirTemp(baseDir, "notable-files-unit-test")
		if err != nil {
			return nil, nil, err
		}
		dirs = append(dirs, dir)

		for j := 0; j < filesPerDir; j++ {
			filePath := fmt.Sprintf("%s/%s", dir, ulid.New())
			file, err := os.Create(filePath)
			if err != nil {
				return nil, nil, err
			}
			defer file.Close()
			files = append(files, filePath)
		}
	}

	sort.Strings(files)
	return dirs, files, nil
}
