package filewalker

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"
)

// duration is a thin wrapper around time.Duration allowing for marshalling/unmarshalling
// durations as strings, rather than the default of nanoseconds.
type duration time.Duration

func (d duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("unmarshalling duration: %w", err)
	}
	parsedDuration, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("parsing duration: %w", err)
	}
	*d = duration(parsedDuration)
	return nil
}

// fileTypeFilter is an optional component of the filewalk configuration.
// When it is set, it allows for restricting filewalk results to only files
// or only directories.
type fileTypeFilter struct {
	name    string
	matches func(f fs.FileMode) bool
}

const (
	fileTypeFile = "file"
	fileTypeDir  = "dir"
)

func (ft *fileTypeFilter) String() string {
	if ft == nil {
		return ""
	}
	return ft.name
}

func (ft fileTypeFilter) MarshalJSON() ([]byte, error) {
	return json.Marshal(ft.name)
}

func (ft *fileTypeFilter) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return fmt.Errorf("unmarshalling string: %w", err)
	}

	switch s {
	case fileTypeFile:
		ft.name = fileTypeFile
		ft.matches = func(f fs.FileMode) bool {
			return !f.IsDir()
		}
		return nil
	case fileTypeDir:
		ft.name = fileTypeDir
		ft.matches = func(f fs.FileMode) bool {
			return f.IsDir()
		}
		return nil
	default:
		return fmt.Errorf("unsupported file filter type %s", s)
	}
}

type (
	// filewalkConfig is the configuration for an individual filewalker.
	filewalkConfig struct {
		WalkInterval duration `json:"walk_interval"`
		filewalkDefinition
		Overlays []filewalkConfigOverlay `json:"overlays"`
	}

	// filewalkConfigOverlay will override any settings in filewalkConfig, if its Filters
	// apply to this launcher installation. (This allows the cloud to provide one filewalkConfig with
	// an overlay for each individual OS, allowing for setting OS-specific paths, etc.)
	filewalkConfigOverlay struct {
		Filters map[string]string `json:"filters"` // determines if this overlay is applicable to this launcher installation
		filewalkDefinition
	}

	// filewalkDefinition is the configuration shared between the base filewalkConfig and the overlays --
	// these are the settings that can be overridden via overlay.
	filewalkDefinition struct {
		RootDirs       *[]string         `json:"root_dirs,omitempty"`
		FileNameRegex  *regexp.Regexp    `json:"file_name_regex,omitempty"`
		SkipDirs       *[]*regexp.Regexp `json:"skip_dirs,omitempty"`
		FileTypeFilter *fileTypeFilter   `json:"file_type_filter,omitempty"`
	}
)

type (
	// lines up with a single filewalkConfig by name
	matcher struct {
		name     string           // the server's filewalkConfig.name
		skipDirs []*regexp.Regexp // directories excluded from walk
		fileName *regexp.Regexp   // filenames included in results
		fileType *fileTypeFilter  // file types excluded
	}

	// specification for a single walk run
	walkSpec struct {
		roots    []string              // the unique, top-level absolute paths to walk
		activate map[string][]*matcher // uppermost path => all new matchers applicable
		matchers []*matcher
	}
)

// resolve realizes all passed server filewalkConfigs into a walkSpec that associates
// which filtered results belong to which filewalkConfig by name.
//
// resolve uses the filesystem to evaluate the root paths globs, then deduplicates
// paths among the matchers which share them.
func resolve(ctx context.Context, slogger *slog.Logger, cfgs map[string]filewalkConfig, goos string) walkSpec {
	spec := walkSpec{activate: make(map[string][]*matcher)}
	pathsToWalk := make(map[string]string)
	var canonicalRoots []string

	for _, name := range slices.Sorted(maps.Keys(cfgs)) {
		cfg := cfgs[name]
		matcher := &matcher{name: name}
		var rootDirs []string

		// simple shortcut for overlay application
		apply := func(def filewalkDefinition) {
			if def.RootDirs != nil {
				rootDirs = *def.RootDirs
			}
			if def.FileNameRegex != nil {
				matcher.fileName = def.FileNameRegex
			}
			if def.SkipDirs != nil {
				matcher.skipDirs = *def.SkipDirs
			}
			if def.FileTypeFilter != nil {
				matcher.fileType = def.FileTypeFilter
			}
		}

		apply(cfg.filewalkDefinition)
		for _, overlay := range cfg.Overlays {
			if overlayFiltersMatch(overlay.Filters, goos) {
				apply(overlay.filewalkDefinition)
			}
		}

		spec.matchers = append(spec.matchers, matcher)

		for _, rootDir := range rootDirs {
			globbedPaths, err := filepath.Glob(rootDir)
			if err != nil {
				slogger.Log(ctx, slog.LevelWarn,
					"error globbing for directories",
					"filewalker_name", name,
					"root_dir", rootDir,
					"err", err,
				)
				continue
			}

			for _, path := range globbedPaths {
				canonicalRoot := canonicalize(path)
				if !slices.Contains(spec.activate[canonicalRoot], matcher) {
					spec.activate[canonicalRoot] = append(spec.activate[canonicalRoot], matcher)
				}
				if _, ok := pathsToWalk[canonicalRoot]; !ok {
					pathsToWalk[canonicalRoot] = path
				}
			}
		}
	}

	// Parent paths sort before their longer children, which allows deduping
	// our dense map of pathsToWalk.
	//
	// NB: there are clever ways to remove this. Don't. This is hard enough to
	// track as-is.
	for _, root := range slices.Sorted(maps.Keys(pathsToWalk)) {
		if !slices.ContainsFunc(canonicalRoots, func(otherRoot string) bool { return strings.HasPrefix(root, otherRoot) }) {
			canonicalRoots = append(canonicalRoots, root)
			spec.roots = append(spec.roots, pathsToWalk[root])
		}
	}

	return spec
}

func overlayFiltersMatch(overlayFilters map[string]string, goos string) bool {
	// Currently, the only filter we expect is for OS.
	if filterGoos, goosFound := overlayFilters["goos"]; goosFound {
		return filterGoos == goos
	}
	return false
}

const pathSep = string(filepath.Separator)

// canonicalize a path so they match when walked and configured.
// As a side effect, if a root is a file we store it with a slash on the end.
func canonicalize(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return strings.TrimSuffix(path, pathSep) + pathSep
}
