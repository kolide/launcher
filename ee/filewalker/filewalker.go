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
	"time"

	"github.com/kolide/launcher/v2/ee/agent/types"
	"github.com/kolide/launcher/v2/ee/observability"
)

// filewalker stores a configured walk definition and where the results go
type filewalker struct {
	name string   // the name of this walker
	spec walkSpec // what to walk

	slogger      *slog.Logger
	resultsStore types.GetterSetterDeleter
}

func newFilewalker(spec walkSpec, resultsStore types.GetterSetterDeleter, slogger *slog.Logger) *filewalker {
	name := spec.matchers[0].name // TODO: intermediate
	return &filewalker{
		name:         name,
		spec:         spec,
		slogger:      slogger.With("filewalker_name", name),
		resultsStore: resultsStore,
	}
}

// Filewalk executes a filewalk with the configured settings, and then stores the results and walk time. Context
// is respected and interrupts walks and result storage.
// TODO: intermediate commit. Only executes the first matcher.
func (f *filewalker) Filewalk(ctx context.Context) {
	ctx, span := observability.StartSpan(ctx, "filewalk_name", f.name)
	defer span.End()

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
			if ctx.Err() != nil {
				f.slogger.Log(ctx, slog.LevelDebug, "file walk interrupted by context error")
				return filepath.SkipAll
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
		if ctx.Err() != nil {
			return
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
