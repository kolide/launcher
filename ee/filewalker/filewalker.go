package filewalker

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"sync"

	"github.com/charlievieth/fastwalk"
)

func Filewalk(ctx context.Context, startDir string) ([]string, error) {
	fileNames := make([]string, 0)
	if err := filepath.WalkDir(startDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Ignore permissions errors
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return err
		}

		if d.IsDir() {
			return nil
		}

		fileNames = append(fileNames, path)

		return nil
	}); err != nil {
		return nil, fmt.Errorf("walking %s: %w", startDir, err)
	}

	return fileNames, nil
}

func FastwalkWithChannel(ctx context.Context, startDir string) ([]string, error) {
	var wg sync.WaitGroup
	fileNames := make([]string, 0)
	filenamesChan := make(chan string, 1000)
	wg.Go(func() {
		for {
			filename, ok := <-filenamesChan
			if !ok {
				return
			}
			fileNames = append(fileNames, filename)
		}
	})

	if err := fastwalk.Walk(&fastwalk.Config{
		Follow: false, // don't follow symlinks
	}, startDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}

		filenamesChan <- path

		return nil
	}); err != nil {
		return nil, fmt.Errorf("walking %s: %w", startDir, err)
	}

	close(filenamesChan)
	wg.Wait()

	return fileNames, nil
}

func FastwalkWithLock(ctx context.Context, startDir string) ([]string, error) {
	fileNames := make([]string, 0)
	fileNamesLock := &sync.Mutex{}

	if err := fastwalk.Walk(&fastwalk.Config{
		Follow: false, // don't follow symlinks
	}, startDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}

		fileNamesLock.Lock()
		fileNames = append(fileNames, path)
		fileNamesLock.Unlock()

		return nil
	}); err != nil {
		return nil, fmt.Errorf("walking %s: %w", startDir, err)
	}

	return fileNames, nil
}
