package filewalker

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charlievieth/fastwalk"
	"github.com/kolide/launcher/ee/agent/types"
)

type filewalkConfig struct {
	rootDir       string
	fileNameRegex *regexp.Regexp
	// fileType      fs.FileMode
}

type Filewalker struct {
	walkInterval  time.Duration
	walkTicker    *time.Ticker
	configs       map[string]filewalkConfig // map table name to config
	filewalksLock *sync.Mutex
	filewalks     map[string][]string // map table name to filepaths

	// Internals
	k       types.Knapsack
	slogger *slog.Logger

	// Handle actor shutdown
	interrupt   chan struct{}
	interrupted *atomic.Bool
}

func NewFilewalker(k types.Knapsack, slogger *slog.Logger, walkInterval time.Duration) *Filewalker {
	return &Filewalker{
		walkInterval:  walkInterval,
		filewalksLock: &sync.Mutex{},
		k:             k,
		slogger:       slogger.With("component", "filewalker"),
		interrupt:     make(chan struct{}, 10), // We have a buffer so we don't block on sending to this channel
		interrupted:   &atomic.Bool{},
	}
}

func (f *Filewalker) Execute() error {
	f.walkTicker = time.NewTicker(f.walkInterval)
	defer f.walkTicker.Stop()

	for {
		for tableName, config := range f.configs {
			// fastwalk already has an optimized number of workers, so we want to perform filewalks
			// for each table in sequence, rather than in parallel.
			results, err := f.filewalk(context.TODO(), config.rootDir, config.fileNameRegex)
			if err != nil {
				f.slogger.Log(context.TODO(), slog.LevelError,
					"could not filewalk",
					"table_name", tableName,
					"err", err,
				)
				continue
			}
			f.filewalksLock.Lock()
			f.filewalks[tableName] = results
			f.filewalksLock.Unlock()
		}

		select {
		case <-f.interrupt:
			f.slogger.Log(context.TODO(), slog.LevelDebug,
				"received external interrupt, stopping",
			)
			return nil
		case <-f.walkTicker.C:
			continue
		}
	}
}

func (f *Filewalker) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if f.interrupted.Swap(true) {
		return
	}

	f.interrupt <- struct{}{}
}

func (f *Filewalker) filewalk(ctx context.Context, startDir string, fileNameRegex *regexp.Regexp) ([]string, error) {
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

	if err := fastwalk.Walk(&fastwalk.DefaultConfig, startDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			f.slogger.Log(ctx, slog.LevelWarn,
				"error while filewalking",
				"start_dir", startDir,
				"path", path,
				"err", err,
			)
			return nil
		}
		if d.IsDir() {
			return nil
		}

		if fileNameRegex == nil || fileNameRegex.MatchString(filepath.Base(path)) {
			filenamesChan <- path
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("walking %s: %w", startDir, err)
	}

	close(filenamesChan)
	wg.Wait()

	return fileNames, nil
}
