package checkpoint

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilesInDirs(t *testing.T) {
	t.Parallel()

	// setup various test directories
	tempDir := t.TempDir()
	doesntExistDir := filepath.Join(tempDir, "doesnt_exist")
	emptyDir, err := os.MkdirTemp(tempDir, "empty_dir")
	require.NoError(t, err)
	dirWithFiles, err := os.MkdirTemp(tempDir, "dir_with_files")
	require.NoError(t, err)

	for j := 1; j < 3; j++ {
		filePath := filepath.Join(dirWithFiles, fmt.Sprintf("file%d", j))
		file, err := os.Create(filePath)
		require.NoError(t, err, fmt.Errorf("creating file: %w:", err))
		file.Close()
	}

	results := filesInDirs(doesntExistDir, emptyDir, dirWithFiles)

	var tests = []struct {
		name     string
		dir      string
		expected []string
	}{
		{
			name:     "doesnt_exist",
			dir:      doesntExistDir,
			expected: []string{"not present"},
		},
		{
			name:     "empty",
			dir:      emptyDir,
			expected: []string{"present, but empty"},
		},

		{
			name:     "dir_with_files",
			dir:      dirWithFiles,
			expected: []string{"contains", "file1", "file2"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			actual := results[tt.dir]

			for _, expected := range tt.expected {
				assert.Contains(t, actual, expected)
			}
		})
	}
}
