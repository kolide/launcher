package filewalker

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/fs"
	"iter"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kolide/launcher/v2/ee/agent/types"
	"github.com/kolide/launcher/v2/ee/observability"
	kolidefilepath "github.com/kolide/launcher/v2/pkg/filepath"
)

// filewalker stores a configured walk definition and where the results go
type filewalker struct {
	name string   // the name of this walker
	spec walkSpec // what to walk

	slogger      *slog.Logger
	resultsStore types.GetterSetterDeleter
}

func newFilewalker(spec walkSpec, resultsStore types.GetterSetterDeleter, slogger *slog.Logger) *filewalker {
	names := make([]string, 0, len(spec.matchers))
	for _, m := range spec.matchers {
		names = append(names, m.name)
	}
	name := strings.Join(names, ",")
	return &filewalker{
		name:         name,
		spec:         spec,
		slogger:      slogger.With("filewalker_name", name),
		resultsStore: resultsStore,
	}
}

// Filewalk executes the configured walkSpec once and stores the results in resultStore. Context
// is respected and interrupts walks and result storage.
func (f *filewalker) Filewalk(ctx context.Context) {
	ctx, span := observability.StartSpan(ctx, "filewalk_name", f.name)
	defer span.End()

	walkStart := time.Now()
	results := make(map[string][]string, len(f.spec.matchers))
	for _, m := range f.spec.matchers {
		results[m.name] = make([]string, 0)
	}
	errorCounts := make(map[string]int)
	var pathsWalked, dirsSkipped int

	for _, match := range f.spec.roots {
		// filepath.WalkDir, but directories return a state which is passed to every child of that directory walked.
		// Approach below uses state to track for a folder which matchers are active or can become active.
		//
		// The depth-first walk terminates when all matchers are removed and no roots in walkSpec specify matchers
		// for a child path.
		if err := kolidefilepath.WalkDirWithState(match, newMatcherSet(), func(path string, d fs.DirEntry, err error, activeMatchers matcherSet) (matcherSet, error) {
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
				return newMatcherSet(), nil
			}
			if ctx.Err() != nil {
				f.slogger.Log(ctx, slog.LevelDebug, "file walk interrupted by context error")
				return matcherSet{}, filepath.SkipAll
			}

			pathsWalked++

			// activate holds all top-most paths at which matchers apply. They get added here from fileSpec
			// and pruned below.
			// NB: a file can activate a matcher.
			activeMatchers = activeMatchers.with(f.spec.activate[canonicalize(path)])

			// Prune matchers which skip this folder's children. Descent to children can stop here
			// when all matchers are exhausted and our spec has no additional roots underneath with more.
			if d.IsDir() {
				activeMatchers = activeMatchers.withoutSkipping(path)
				// Keep descending with nothing active if another config's root lies below.
				if activeMatchers.empty() && !f.spec.hasRootBelow(path) {
					dirsSkipped++
					return newMatcherSet(), fs.SkipDir
				}
			}

			// Add this file to our results.
			for m := range activeMatchers.matching(path, d) {
				results[m.name] = append(results[m.name], path)
			}
			return activeMatchers, nil
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

	filesMatched := make(map[string]int, len(results))
	for _, m := range f.spec.matchers {
		f.storeResults(ctx, m.name, results[m.name])
		filesMatched[m.name] = len(results[m.name])
	}

	span.AddEvent("walk_results_stored")

	f.slogger.Log(ctx, slog.LevelInfo,
		"completed filewalk",
		"walk_duration", time.Since(walkStart).String(),
		"error_counts", errorCounts,
		"paths_walked", pathsWalked,
		"dirs_skipped", dirsSkipped,
		"files_matched", filesMatched,
	)
}

// storeResults stores one matcher's results, then its last walk time.
func (f *filewalker) storeResults(ctx context.Context, name string, paths []string) {
	resultsRaw, err := json.Marshal(paths)
	if err != nil {
		f.slogger.Log(ctx, slog.LevelError,
			"could not marshal filewalk results for storage",
			"err", err,
		)
		return
	}
	if err := f.resultsStore.Set([]byte(name), resultsRaw); err != nil {
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
	if err := f.resultsStore.Set(LastWalkTimeKey(name), lastWalkTimeBuffer.Bytes()); err != nil {
		f.slogger.Log(ctx, slog.LevelError,
			"could not set last walk time in storage",
			"err", err,
		)
	}
}

// LastWalkTimeKey gives the key to query the results store to retrieve the last walk time for the given filewalker.
func LastWalkTimeKey(filewalkName string) []byte {
	return fmt.Appendf(nil, "%s_last_walk", filewalkName)
}

// matcherSet is never modified in place; methods return a new set when it changes.
// Sets are shared between a directory and its children during a walk.
type matcherSet struct{ ms []*matcher }

func newMatcherSet(ms ...*matcher) matcherSet {
	return matcherSet{}.with(matcherSet{ms})
}

// empty returns true when the set is empty.
func (s matcherSet) empty() bool { return len(s.ms) == 0 }

// with returns a copy of itself with the passed matchers, if any, included.
func (s matcherSet) with(add matcherSet) matcherSet {
	if add.empty() {
		return s
	}

	// Most calls in a walk add nothing. O(N^2) on low N is free
	// compared to a walk.
	out := slices.Clone(s.ms)
	for _, m := range add.ms {
		if !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	return matcherSet{out}
}

// withoutSkipping returns a copy of itself without matchers which skip the passed directory.
func (s matcherSet) withoutSkipping(dir string) matcherSet {
	skips := func(m *matcher) bool { return m.shouldSkipDir(dir) }
	if !slices.ContainsFunc(s.ms, skips) {
		return s
	}
	return matcherSet{slices.DeleteFunc(slices.Clone(s.ms), skips)}
}

// matching returns all matchers in the set which match the DirEntry.
func (s matcherSet) matching(path string, d fs.DirEntry) iter.Seq[*matcher] {
	return func(yield func(*matcher) bool) {
		for _, m := range s.ms {
			if m.matches(path, d) && !yield(m) {
				return
			}
		}
	}
}

// shouldSkipDir returns true if any skipDirs matches this directory.
func (m *matcher) shouldSkipDir(dir string) bool {
	for _, skipDirRegex := range m.skipDirs {
		if skipDirRegex.MatchString(dir) {
			return true
		}
	}
	return false
}

func (m *matcher) matches(path string, d fs.DirEntry) bool {
	if m.fileType != nil && !m.fileType.matches(d.Type()) {
		return false
	}

	if m.fileName != nil && !m.fileName.MatchString(filepath.Base(path)) {
		return false
	}

	return true
}
