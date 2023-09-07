package checkups

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_quarantine_checkDirs(t *testing.T) {
	t.Parallel()

	const folderKeyword = "quarantine_checkup_test"

	tests := []struct {
		name                string
		shouldPass          bool
		pathsFunc           func(t *testing.T) (string, map[string][]string)
		maxDepth            int
		expectedDirsChecked int
	}{
		{
			name: "found quarantined files",
			pathsFunc: func(t *testing.T) (string, map[string][]string) {
				dir := t.TempDir()

				require.NoError(t, os.MkdirAll(filepath.Join(dir, "1", folderKeyword, "2", "3", "4"), 0755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "1", folderKeyword, "someFile"), nil, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "1", folderKeyword, "anotherFile"), nil, 0755))

				require.NoError(t, os.WriteFile(filepath.Join(dir, "1", folderKeyword, "2", "3", "yetAnotherFile"), nil, 0755))
				return dir, map[string][]string{
					filepath.Join(dir, "1", folderKeyword):                {"someFile", "anotherFile"},
					filepath.Join(dir, "1", folderKeyword, "2"):           {},
					filepath.Join(dir, "1", folderKeyword, "2", "3"):      {"yetAnotherFile"},
					filepath.Join(dir, "1", folderKeyword, "2", "3", "4"): {},
				}
			},
			maxDepth:            10,
			expectedDirsChecked: 6,
		},
		{
			name: "doesnt exceed max depth",
			pathsFunc: func(t *testing.T) (string, map[string][]string) {
				dir := t.TempDir()

				require.NoError(t, os.MkdirAll(filepath.Join(dir, "1", "2", folderKeyword), 0755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "1", "not in special folder"), nil, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "1", "2", folderKeyword, "somefile"), nil, 0755))
				return dir, map[string][]string{}
			},
			maxDepth:            2,
			expectedDirsChecked: 3,
		},
		{
			name: "no notable files",
			pathsFunc: func(t *testing.T) (string, map[string][]string) {
				dir := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(dir, "1", "2", "3"), 0755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "1", "2", "dont care"), nil, 0755))
				return dir, map[string][]string{}
			},
			maxDepth:            10,
			expectedDirsChecked: 4,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			q := quarantine{
				quarantineDirPathFilenames: make(map[string][]string),
			}
			rootPath, expected := tt.pathsFunc(t)
			q.checkDirs(io.Discard, 0, tt.maxDepth, rootPath, folderKeyword)

			for path, files := range expected {
				val, ok := q.quarantineDirPathFilenames[path]
				require.True(t, ok, "path should be present in quarantine")
				require.ElementsMatch(t, files, val)
			}
		})
	}
}
