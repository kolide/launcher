package filepath

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

var walkDirTestTree = fstest.MapFS{
	"a/b/file":  {},
	"a/c/file":  {},
	"a/file":    {},
	"d/e/f/g":   {},
	"top-level": {},
}

// WalkDirWithState should visit the same paths in the same order as filepath.WalkDir.
func TestWalkDirWithState_MatchesWalkDir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.CopyFS(root, walkDirTestTree))

	var want []string
	require.NoError(t, filepath.WalkDir(root, func(path string, _ fs.DirEntry, err error) error {
		want = append(want, path)
		return err
	}))

	var got []string
	require.NoError(t, WalkDirWithState(root, struct{}{}, func(path string, _ fs.DirEntry, err error, state struct{}) (struct{}, error) {
		got = append(got, path)
		return state, err
	}))

	require.Equal(t, want, got)
}

// Children should receive the state their parent returned, unaffected by siblings.
func TestWalkDirWithState_ChildrenReceiveParentState(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.CopyFS(root, walkDirTestTree))

	depths := make(map[string]int)
	require.NoError(t, WalkDirWithState(root, 0, func(path string, _ fs.DirEntry, err error, depth int) (int, error) {
		rel, relErr := filepath.Rel(root, path)
		require.NoError(t, relErr)
		depths[filepath.ToSlash(rel)] = depth
		return depth + 1, err
	}))

	require.Equal(t, map[string]int{
		".":         0,
		"a":         1,
		"a/b":       2,
		"a/b/file":  3,
		"a/c":       2,
		"a/c/file":  3,
		"a/file":    2,
		"d":         1,
		"d/e":       2,
		"d/e/f":     3,
		"d/e/f/g":   4,
		"top-level": 1,
	}, depths)
}

// SkipDir should prune a directory and SkipAll should stop the walk, both without an error.
func TestWalkDirWithState_Skips(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.CopyFS(root, walkDirTestTree))

	var visited []string
	require.NoError(t, WalkDirWithState(root, struct{}{}, func(path string, d fs.DirEntry, err error, state struct{}) (struct{}, error) {
		rel, relErr := filepath.Rel(root, path)
		require.NoError(t, relErr)
		visited = append(visited, filepath.ToSlash(rel))
		switch filepath.ToSlash(rel) {
		case "a":
			return state, filepath.SkipDir
		case "d/e":
			return state, filepath.SkipAll
		}
		return state, err
	}))

	require.Equal(t, []string{".", "a", "d", "d/e"}, visited)
}
