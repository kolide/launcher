package filewalker

import (
	"context"
	"io/fs"
	"log/slog"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/charlievieth/fastwalk"
	"github.com/kolide/launcher/ee/gowrapper"
)

type filewalkConfig struct {
	name          string
	walkInterval  time.Duration
	rootDir       string
	fileNameRegex *regexp.Regexp
	// fileType      fs.FileMode
}

type filewalker struct {
	cfg       filewalkConfig
	slogger   *slog.Logger
	ticker    *time.Ticker
	walkLock  *sync.Mutex
	results   []string
	interrupt chan struct{}
}

func newFilewalker(cfg filewalkConfig, slogger *slog.Logger) *filewalker {
	return &filewalker{
		cfg:       cfg,
		slogger:   slogger.With("filewalker_name", cfg.name),
		walkLock:  &sync.Mutex{},
		results:   make([]string, 0),       // TODO RM: init from storage
		interrupt: make(chan struct{}, 10), // We have a buffer so we don't block on sending to this channel
	}
}

func (f *filewalker) Work() {
	f.ticker = time.NewTicker(f.cfg.walkInterval)
	defer f.ticker.Stop()

	for {
		f.filewalk(context.TODO())

		select {
		case <-f.interrupt:
			f.slogger.Log(context.TODO(), slog.LevelDebug,
				"received external interrupt, stopping",
			)
			return
		case <-f.ticker.C:
			continue
		}
	}
}

func (f *filewalker) Stop() {
	f.interrupt <- struct{}{}
}

func (f *filewalker) Paths() []string {
	f.walkLock.Lock()
	defer f.walkLock.Unlock()

	return f.results
}

func (f *filewalker) UpdateConfig(newCfg filewalkConfig) {
	f.walkLock.Lock()
	defer f.walkLock.Unlock()

	if newCfg.walkInterval != f.cfg.walkInterval && f.ticker != nil {
		f.ticker.Reset(newCfg.walkInterval)
	}

	f.cfg = newCfg
}

func (f *filewalker) filewalk(ctx context.Context) {
	f.walkLock.Lock()
	defer f.walkLock.Unlock()

	fileNames := make([]string, 0)
	filenamesChan := make(chan string, 1000)
	var wg sync.WaitGroup
	wg.Add(1)
	gowrapper.Go(ctx, f.slogger, func() {
		defer wg.Done()
		for {
			filename, ok := <-filenamesChan
			if !ok {
				return
			}
			fileNames = append(fileNames, filename)
		}
	})

	if err := fastwalk.Walk(&fastwalk.DefaultConfig, f.cfg.rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			f.slogger.Log(ctx, slog.LevelWarn,
				"error while filewalking",
				"start_dir", f.cfg.rootDir,
				"path", path,
				"err", err,
			)
			return nil
		}
		if d.IsDir() {
			return nil
		}

		if f.cfg.fileNameRegex == nil || f.cfg.fileNameRegex.MatchString(filepath.Base(path)) {
			filenamesChan <- path
		}

		return nil
	}); err != nil {
		f.slogger.Log(ctx, slog.LevelError,
			"could not complete filewalk",
			"err", err,
		)
		close(filenamesChan)
		return
	}

	close(filenamesChan)
	wg.Wait()

	f.results = fileNames
}
