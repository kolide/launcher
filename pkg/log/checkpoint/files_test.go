package checkpoint

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/kolide/kit/ulid"
	"github.com/stretchr/testify/require"
)

func TestFilesFound(t *testing.T) {
	t.Parallel()

	tempDir, err := os.MkdirTemp("", "log-checkpoint-files-test")
	require.NoError(t, err, "making temp dir")
	defer os.RemoveAll(tempDir)

	var tests = []struct {
		dirsToCreate int
		filesPerDir  int
	}{
		{dirsToCreate: 2, filesPerDir: 2},
		// doesn't search any folders since "dirsToSerach" below will be empty array
		{dirsToCreate: 0, filesPerDir: 0},
	}

	for _, tt := range tests {
		t.Run("testFilesFound", func(t *testing.T) {
			t.Parallel()

			dirsToSearch, expectedPaths, err := createTestFiles(tempDir, tt.dirsToCreate, tt.filesPerDir)
			require.NoError(t, err, "creating test files")

			foundPaths := fileNamesInDirs(dirsToSearch...)
			sort.Strings(foundPaths)
			require.Equal(t, expectedPaths, foundPaths)
			require.Equal(t, tt.dirsToCreate*tt.filesPerDir, len(foundPaths))
		})
	}
}

func TestDirNotFound(t *testing.T) {
	t.Parallel()

	tempDir, err := os.MkdirTemp("", "log-checkpoint-files-test")
	require.NoError(t, err, "making temp dir")
	defer os.RemoveAll(tempDir)

	nonExistantDirs := []string{
		filepath.Join(tempDir, ulid.New()),
		filepath.Join(tempDir, ulid.New()),
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
			pathErr.Err = errors.New("The system cannot find the file specified.")
		}

		expectedOutput = append(expectedOutput, pathErr.Error())
	}

	foundPaths := fileNamesInDirs(nonExistantDirs...)

	require.Equal(t, expectedOutput, foundPaths)
	require.Equal(t, len(nonExistantDirs), len(foundPaths))
}

func TestDirEmpty(t *testing.T) {
	t.Parallel()

	tempDir, err := os.MkdirTemp("", "log-checkpoint-files-test")
	require.NoError(t, err, "making temp dir")
	defer os.RemoveAll(tempDir)

	dirs, _, err := createTestFiles(tempDir, 2, 0)
	require.NoError(t, err, "creating test dirs")

	expectedOutput := []string{}
	for _, dir := range dirs {
		expectedOutput = append(expectedOutput, emptyDirMsg(dir))
	}

	foundPaths := fileNamesInDirs(dirs...)

	require.Equal(t, expectedOutput, foundPaths)
	require.Equal(t, len(dirs), len(foundPaths))
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
			filePath := filepath.Join(dir, ulid.New())
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
