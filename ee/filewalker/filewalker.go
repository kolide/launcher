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
	"sync"
	"time"

	"github.com/kolide/launcher/v2/ee/agent/types"
	"github.com/kolide/launcher/v2/ee/observability"
)

// filewalker performs filewalks at the configured interval, storing results in its resultsStore.
type filewalker struct {
	// Configuration
	name         string
	walkInterval time.Duration
	spec         walkSpec

	// Internals
	slogger      *slog.Logger
	ticker       *time.Ticker
	walkLock     *sync.Mutex
	resultsStore types.GetterSetterDeleter

	// Handle shutdown
	interrupt chan struct{}
}

func newFilewalker(name string, walkInterval time.Duration, spec walkSpec, resultsStore types.GetterSetterDeleter, slogger *slog.Logger) *filewalker {
	fw := &filewalker{
		name:         name,
		walkInterval: walkInterval,
		slogger:      slogger.With("filewalker_name", name),
		walkLock:     &sync.Mutex{},
		resultsStore: resultsStore,
		interrupt:    make(chan struct{}, 10), // We have a buffer so we don't block on sending to this channel
	}

	// Set config options from cfg
	fw.UpdateConfig(walkInterval, spec)

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
		f.Filewalk(context.TODO())

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
	} else {
		f.slogger.Log(context.TODO(), slog.LevelInfo,
			"removed stored results for filewalk",
		)
	}
	f.Stop()
}

func (f *filewalker) Stop() {
	f.interrupt <- struct{}{}
}

func (f *filewalker) UpdateConfig(walkInterval time.Duration, spec walkSpec) {
	f.walkLock.Lock()
	defer f.walkLock.Unlock()

	// Update walk interval first, updating ticker if it exists
	if walkInterval != f.walkInterval && f.ticker != nil {
		f.ticker.Reset(walkInterval)
	}
	f.walkInterval = walkInterval
	f.spec = spec

	m := f.spec.matchers[0]
	f.slogger.Log(context.TODO(), slog.LevelInfo,
		"set filewalker config",
		"walk_interval", f.walkInterval.String(),
		"root_dirs", f.spec.roots,
		"file_name_regex", m.fileName,
		"file_type_filter", m.fileType.String(),
		"skip_dirs", m.skipDirs,
	)
}

// Filewalk executes a filewalk with the configured settings, and then stores the results and walk time.
func (f *filewalker) Filewalk(ctx context.Context) {
	ctx, span := observability.StartSpan(ctx, "filewalk_name", f.name)
	defer span.End()

	f.walkLock.Lock()
	defer f.walkLock.Unlock()

	span.AddEvent("walk_lock_acquired")

	walkStart := time.Now()
	fileNames := make([]string, 0)
	errorCounts := make(map[string]int)
	var pathsWalked, dirsSkipped int

	m := f.spec.matchers[0]
	for _, match := range f.spec.roots {
		if err := filepath.WalkDir(match, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				key, expected := classifyWalkError(err)
				errorCounts[key]++
				if !expected {
					f.slogger.Log(ctx, slog.LevelWarn,
						"error while filewalking",
						"start_dir", match,
						"path", path,
						"err", err,
					)
				}
				return nil
			}

			pathsWalked++

			// Prune skipped directories before any other filter, so that we don't descend unnecessarily
			if d.IsDir() && f.shouldSkipDir(path) {
				dirsSkipped++
				return fs.SkipDir
			}

			if m.fileType != nil && !m.fileType.matches(d.Type()) {
				return nil
			}

			if m.fileName != nil && !m.fileName.MatchString(filepath.Base(path)) {
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

	span.AddEvent("walk_complete")

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

	span.AddEvent("walk_results_stored")

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

	span.AddEvent("walk_time_stored")

	f.slogger.Log(ctx, slog.LevelInfo,
		"completed filewalk",
		"walk_duration", time.Since(walkStart).String(),
		"error_counts", errorCounts,
		"paths_walked", pathsWalked,
		"dirs_skipped", dirsSkipped,
		"files_matched", len(fileNames),
	)
}

// LastWalkTimeKey gives the key to query the results store to retrieve the last walk time for the given filewalker.
func LastWalkTimeKey(filewalkName string) []byte {
	return fmt.Appendf(nil, "%s_last_walk", filewalkName)
}

func (f *filewalker) shouldSkipDir(dir string) bool {
	for _, skipDirRegex := range f.spec.matchers[0].skipDirs {
		if skipDirRegex.MatchString(dir) {
			return true
		}
	}
	return false
}
