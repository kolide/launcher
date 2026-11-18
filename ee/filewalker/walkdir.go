package filewalker

import (
	"io/fs"
	"path/filepath"
)

// walkDirWithState wraps filepath.WalkDir with the same guarantees. It also passes the returned state (S)
// for d.IsDir() to any child paths.
//
// If os.ReadDir fails within WalkDir, fn is called twice like WalkDir: once with a nil error and a second
// time with the ReadDir error. The second call's returned state will be passed to any calls for partially read directory contents.
func walkDirWithState[S any](root string, initial S, fn func(path string, d fs.DirEntry, err error, state S) (S, error)) error {
	type frame struct {
		dir   string
		state S
	}
	var stack []frame

	// Pass the full path to the current directory. Pops the stack if needed until its top frame is
	// for dir, then returns the state for the passed dir.
	stateFor := func(dir string) S {
		for len(stack) > 0 {
			top := stack[len(stack)-1]
			if top.dir == dir {
				return top.state
			}
			stack = stack[:len(stack)-1]
		}
		return initial
	}

	return filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		state, err := fn(path, d, walkErr, stateFor(filepath.Dir(path)))
		// Directories will be recursed, causing state to be tracked only if there's no error.
		if err == nil && d != nil && d.IsDir() {
			stack = append(stack, frame{dir: filepath.Clean(path), state: state})
		}
		return err
	})
}
