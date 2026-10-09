// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package filepath

// Adapted from filepath.WalkDir (Go 1.26). The only change is threading state: fn returns the
// state its directory's children receive. filepath.WalkDir can do this with state tracked outside
// the closure, but passing it through fn is easier to follow when children inherit from parents.

import (
	"io/fs"
	"os"
	"path/filepath"
)

type WalkDirWithStateFunc[S any] func(path string, d fs.DirEntry, err error, state S) (S, error)

func WalkDirWithState[S any](root string, state S, fn WalkDirWithStateFunc[S]) error {
	info, err := os.Lstat(root)
	if err != nil {
		_, err = fn(root, nil, err, state)
	} else {
		err = walkDirWithStateAt(root, fs.FileInfoToDirEntry(info), state, fn)
	}
	if err == filepath.SkipDir || err == filepath.SkipAll {
		return nil
	}
	return err
}

func walkDirWithStateAt[S any](path string, d fs.DirEntry, state S, fn WalkDirWithStateFunc[S]) error {
	childState, err := fn(path, d, nil, state)
	if err != nil || !d.IsDir() {
		if err == filepath.SkipDir && d.IsDir() {
			// Successfully skipped directory.
			err = nil
		}
		return err
	}

	dirs, err := os.ReadDir(path)
	if err != nil {
		// Second call, to report ReadDir error. Receives the same state as the first.
		_, err = fn(path, d, err, state)
		if err != nil {
			if err == filepath.SkipDir && d.IsDir() {
				err = nil
			}
			return err
		}
	}

	for _, d1 := range dirs {
		path1 := filepath.Join(path, d1.Name())
		if err := walkDirWithStateAt(path1, d1, childState, fn); err != nil {
			if err == filepath.SkipDir {
				break
			}
			return err
		}
	}
	return nil
}
