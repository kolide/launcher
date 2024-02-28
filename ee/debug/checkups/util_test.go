package checkups

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_recursiveDirectoryContents(t *testing.T) {
	t.Parallel()

	// Set up a temp dir with some contents to find
	testDir := t.TempDir()
	expectedFiles := []string{"file1", "file2", "file3"}
	for _, fName := range expectedFiles {
		require.NoError(t, os.WriteFile(filepath.Join(testDir, fName), []byte("data"), 0755), "setting up test file")
	}
	secondLevelDir := filepath.Join(testDir, "another")
	require.NoError(t, os.Mkdir(secondLevelDir, 0755), "setting up test dir")
	for _, fName := range expectedFiles {
		require.NoError(t, os.WriteFile(filepath.Join(secondLevelDir, fName), []byte("data"), 0755), "setting up test file")
	}
	expectedTopLevelFileCount := len(expectedFiles) + 1                          // add one for second-level dir
	expectedTotalFileCount := expectedTopLevelFileCount + len(expectedFiles) + 1 // add one for base dir

	// Check we get contents as normal
	contents1 := &bytes.Buffer{}
	topLevelFileCount1, err := recursiveDirectoryContents(contents1, testDir)
	require.NoError(t, err, "did not expect error getting directory contents recursively")
	require.Equal(t, expectedTopLevelFileCount, topLevelFileCount1, "unexpected number of files")
	filesFound1 := strings.Split(strings.ReplaceAll(strings.TrimSpace(contents1.String()), "\r\n", "\n"), "\n")
	require.Equal(t, expectedTotalFileCount, len(filesFound1))

	// Check again to make sure recursiveDirectoryContents handles a trailing slash appropriately
	contents2 := &bytes.Buffer{}
	topLevelFileCount2, err := recursiveDirectoryContents(contents2, testDir+"/")
	require.NoError(t, err, "did not expect error getting directory contents recursively")
	require.Equal(t, expectedTopLevelFileCount, topLevelFileCount2, "unexpected number of files")
	filesFound2 := strings.Split(strings.ReplaceAll(strings.TrimSpace(contents2.String()), "\r\n", "\n"), "\n")
	require.Equal(t, expectedTotalFileCount, len(filesFound2))
}
