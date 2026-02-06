package filewalker

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/charlievieth/fastwalk"
	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/gowrapper"
)

// TODO RM: maybe overlays?
type filewalkConfig struct {
	Name          string         `json:"name"`
	WalkInterval  time.Duration  `json:"walk_interval"`
	RootDir       string         `json:"root_dir"`
	FileNameRegex *regexp.Regexp `json:"file_name_regex"`
	// fileType      fs.FileMode
}

type filewalker struct {
	cfg          filewalkConfig
	slogger      *slog.Logger
	ticker       *time.Ticker
	walkLock     *sync.Mutex
	resultsStore types.GetterSetterDeleter
	interrupt    chan struct{}
}

func newFilewalker(cfg filewalkConfig, resultsStore types.GetterSetterDeleter, slogger *slog.Logger) *filewalker {
	return &filewalker{
		cfg:          cfg,
		slogger:      slogger.With("filewalker_name", cfg.Name),
		walkLock:     &sync.Mutex{},
		resultsStore: resultsStore,
		interrupt:    make(chan struct{}, 10), // We have a buffer so we don't block on sending to this channel
	}
}

func (f *filewalker) Work() {
	f.ticker = time.NewTicker(f.cfg.WalkInterval)
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

func (f *filewalker) Delete() {
	if err := f.resultsStore.Delete([]byte(f.cfg.Name)); err != nil {
		f.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not remove stored results for filewalk during delete",
			"err", err,
		)
	}
	f.Stop()
}

func (f *filewalker) Stop() {
	f.interrupt <- struct{}{}
}

func (f *filewalker) UpdateConfig(newCfg filewalkConfig) {
	f.walkLock.Lock()
	defer f.walkLock.Unlock()

	if newCfg.WalkInterval != f.cfg.WalkInterval && f.ticker != nil {
		f.ticker.Reset(newCfg.WalkInterval)
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

	if err := fastwalk.Walk(&fastwalk.DefaultConfig, f.cfg.RootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			f.slogger.Log(ctx, slog.LevelWarn,
				"error while filewalking",
				"start_dir", f.cfg.RootDir,
				"path", path,
				"err", err,
			)
			return nil
		}
		if d.IsDir() {
			return nil
		}

		if f.cfg.FileNameRegex == nil || f.cfg.FileNameRegex.MatchString(filepath.Base(path)) {
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

	resultsRaw, err := json.Marshal(fileNames)
	if err != nil {
		f.slogger.Log(ctx, slog.LevelError,
			"could not marshal filewalk results for storage",
			"err", err,
		)
		return
	}
	if err := f.resultsStore.Set([]byte(f.cfg.Name), resultsRaw); err != nil {
		f.slogger.Log(ctx, slog.LevelError,
			"could not set filewalk results in storage",
			"err", err,
		)
	}
}
