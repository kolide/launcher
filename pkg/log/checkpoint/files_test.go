package checkpoint

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/kolide/kit/ulid"
	"github.com/stretchr/testify/require"
)

func TestFilesFound(t *testing.T) {
	t.Parallel()

	tempDir, err := os.MkdirTemp("", "log-checkpoint-files-test")
	require.NoError(t, err, "making temp dir")

	t.Cleanup(func() {
		os.RemoveAll(tempDir)
	})

	var tests = []struct {
		name         string
		dirsToCreate int
		filesPerDir  int
	}{
		{name: "2_dirs_2_files", dirsToCreate: 2, filesPerDir: 2},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
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

	tempDir := t.TempDir()

	nonExistantDirs := []string{
		filepath.Join(tempDir, ulid.New()),
		filepath.Join(tempDir, ulid.New()),
	}

	expectedOutput := []string{"No extra osquery files detected"}
	foundPaths := fileNamesInDirs(nonExistantDirs...)
	require.Equal(t, expectedOutput, foundPaths)
}

func TestDirEmpty(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	dirs, _, err := createTestFiles(tempDir, 2, 0)
	require.NoError(t, err, "creating test dirs")

	expectedOutput := []string{"No extra osquery files detected"}
	foundPaths := fileNamesInDirs(dirs...)

	require.Equal(t, expectedOutput, foundPaths)
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
