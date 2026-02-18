package filewalker

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
)

// filewalker performs filewalks at the configured interval, storing results in its resultsStore.
type filewalker struct {
	// Configuration
	name           string
	walkInterval   time.Duration
	rootDirs       []string
	fileNameRegex  *regexp.Regexp
	fileTypeFilter *fileTypeFilter
	skipDirs       []*regexp.Regexp

	// Internals
	slogger      *slog.Logger
	ticker       *time.Ticker
	walkLock     *sync.Mutex
	resultsStore types.GetterSetterDeleter

	// Handle shutdown
	interrupt chan struct{}
}

func newFilewalker(name string, cfg filewalkConfig, resultsStore types.GetterSetterDeleter, slogger *slog.Logger) *filewalker {
	fw := &filewalker{
		name:         name,
		walkInterval: time.Duration(cfg.WalkInterval),
		slogger:      slogger.With("filewalker_name", name),
		walkLock:     &sync.Mutex{},
		resultsStore: resultsStore,
		interrupt:    make(chan struct{}, 10), // We have a buffer so we don't block on sending to this channel
	}

	// Set config options from cfg
	fw.UpdateConfig(cfg)

	return fw
}

// Work executes filewalks on the given interval, until interrupted via Stop.
func (f *filewalker) Work() {
	f.ticker = time.NewTicker(f.walkInterval)
	defer f.ticker.Stop()

	f.slogger.Log(context.TODO(), slog.LevelDebug,
		"starting up",
		"walk_interval", f.walkInterval.String(),
	)

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

// Delete removes all results for a given filewalker from the resultsStore, and then stops the filewalker.
func (f *filewalker) Delete() {
	if err := f.resultsStore.Delete([]byte(f.name)); err != nil {
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

	// Update walk interval first, updating ticker if it exists
	if time.Duration(newCfg.WalkInterval) != f.walkInterval && f.ticker != nil {
		f.ticker.Reset(time.Duration(newCfg.WalkInterval))
	}
	f.walkInterval = time.Duration(newCfg.WalkInterval)

	// Extract root dirs and filename regex from cfg -- applying base options first, and then overlays
	if newCfg.RootDirs != nil {
		f.rootDirs = *newCfg.RootDirs
	}
	if newCfg.FileNameRegex != nil {
		f.fileNameRegex = newCfg.FileNameRegex
	}
	if newCfg.SkipDirs != nil {
		f.skipDirs = *newCfg.SkipDirs
	}
	if newCfg.FileTypeFilter != nil {
		f.fileTypeFilter = newCfg.FileTypeFilter
	}
	for _, overlay := range newCfg.Overlays {
		if !overlayFiltersMatch(overlay.Filters) {
			continue
		}
		if overlay.RootDirs != nil {
			f.rootDirs = *overlay.RootDirs
		}
		if overlay.FileNameRegex != nil {
			f.fileNameRegex = overlay.FileNameRegex
		}
		if overlay.SkipDirs != nil {
			f.skipDirs = *overlay.SkipDirs
		}
		if overlay.FileTypeFilter != nil {
			f.fileTypeFilter = overlay.FileTypeFilter
		}
	}
}

func overlayFiltersMatch(overlayFilters map[string]string) bool {
	// Currently, the only filter we expect is for OS.
	if goos, goosFound := overlayFilters["goos"]; goosFound {
		return goos == runtime.GOOS
	}
	return false
}

// filewalk executes a filewalk with the configured settings, and then stores the results and walk time.
func (f *filewalker) filewalk(ctx context.Context) {
	f.walkLock.Lock()
	defer f.walkLock.Unlock()

	fileNames := make([]string, 0)

	for _, rootDir := range f.rootDirs {
		// rootDir may be a directory, or a glob for a directory.
		matches, err := filepath.Glob(rootDir)
		if err != nil {
			f.slogger.Log(ctx, slog.LevelWarn,
				"error globbing for directories",
				"root_dir", rootDir,
				"err", err,
			)
			continue
		}
		for _, match := range matches {
			if err := filepath.WalkDir(match, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					f.slogger.Log(ctx, slog.LevelWarn,
						"error while filewalking",
						"start_dir", match,
						"path", path,
						"err", err,
					)
					return nil
				}

				// Check to see if we're in a directory that should be skipped
				if f.shouldSkip(path) {
					return fs.SkipDir
				}

				// If our config restricts file type, check that
				if f.fileTypeFilter != nil && !f.fileTypeFilter.matches(d.Type()) {
					return nil
				}

				// Finally, check for the file name regex
				if f.fileNameRegex != nil && !f.fileNameRegex.MatchString(filepath.Base(path)) {
					return nil
				}

				// Add this file to our results
				fileNames = append(fileNames, path)
				return nil
			}); err != nil {
				// Log error, but continue on to process other root dirs
				f.slogger.Log(ctx, slog.LevelError,
					"could not complete filewalk in directory",
					"start_dir", match,
					"err", err,
				)
			}
		}
	}

	resultsRaw, err := json.Marshal(fileNames)
	if err != nil {
		f.slogger.Log(ctx, slog.LevelError,
			"could not marshal filewalk results for storage",
			"err", err,
		)
		return
	}
	if err := f.resultsStore.Set([]byte(f.name), resultsRaw); err != nil {
		f.slogger.Log(ctx, slog.LevelError,
			"could not set filewalk results in storage",
			"err", err,
		)
		return
	}

	// Since we've successfully walked and stored the results, store the last walk time
	lastWalkTimeBuffer := &bytes.Buffer{}
	if err := binary.Write(lastWalkTimeBuffer, binary.NativeEndian, time.Now().Unix()); err != nil {
		f.slogger.Log(ctx, slog.LevelError,
			"could not convert last walk timestamp to bytes",
			"err", err,
		)
		return
	}
	if err := f.resultsStore.Set(LastWalkTimeKey(f.name), lastWalkTimeBuffer.Bytes()); err != nil {
		f.slogger.Log(ctx, slog.LevelError,
			"could not set last walk time in storage",
			"err", err,
		)
	}

	f.slogger.Log(ctx, slog.LevelDebug,
		"completed filewalk",
	)
}

// LastWalkTimeKey gives the key to query the results store to retrieve the last walk time for the given filewalker.
func LastWalkTimeKey(filewalkName string) []byte {
	return fmt.Appendf(nil, "%s_last_walk", filewalkName)
}

func (f *filewalker) shouldSkip(dir string) bool {
	for _, skipDirRegex := range f.skipDirs {
		if skipDirRegex.MatchString(dir) {
			return true
		}
	}
	return false
}
