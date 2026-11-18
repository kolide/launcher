package filewalker

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
	"testing/fstest"
	"time"

	"github.com/kolide/launcher/v2/ee/agent/storage"
	storageci "github.com/kolide/launcher/v2/ee/agent/storage/ci"
	"github.com/kolide/launcher/v2/ee/agent/types"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

// Regression test. We once allowed walking a dir which failed other checks,
// but would have matched a skipdir regex.
//
// SkipDir should prevent walking matching folders.
func TestFilewalk_SkipDirs(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName  string
		fileNameRegex *regexp.Regexp
		fileType      string
	}{
		{testCaseName: "no filters"},
		{testCaseName: "file type filter", fileType: fileTypeFile},
		{testCaseName: "file name regex", fileNameRegex: regexp.MustCompile(`^\.env$`)},
	} {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			fwt := newFilewalkerTest(t)
			fwt.fileType = tt.fileType
			fwt.fileNameRegex = tt.fileNameRegex
			fwt.skipDirs = []*regexp.Regexp{regexp.MustCompile(`node_modules$`)}

			results := fwt.run(t, fstest.MapFS{
				"home/kiwi/.env":                  {},
				"home/kiwi/node_modules/pkg/.env": {},
			})

			require.Contains(t, results, "home/kiwi/.env")
			for _, result := range results {
				require.NotContains(t, result, "node_modules")
			}
		})
	}
}

// Matching files should be returned.
func TestFilewalk_FileNameRegex(t *testing.T) {
	t.Parallel()

	fwt := newFilewalkerTest(t)
	fwt.fileNameRegex = regexp.MustCompile(`^\.env$`)

	results := fwt.run(t, fstest.MapFS{
		"home/kiwi/.env":          {},
		"home/kiwi/a/b/c/.env":    {},
		"home/kiwi/notes.txt":     {},
		"home/kiwi/a/b/c/app.env": {},
	})

	require.ElementsMatch(t, []string{"home/kiwi/.env", "home/kiwi/a/b/c/.env"}, results)
}

// rootDirs globs which find nothing to walk should return nothing successfully.
func TestFilewalk_NoMatchingRootDirs(t *testing.T) {
	t.Parallel()

	fwt := newFilewalkerTest(t)
	fwt.rootDirs = []string{"missing", "missing/*"}

	results := fwt.run(t, fstest.MapFS{
		"home/kiwi/.env": {},
	})

	require.Empty(t, results)
}

// No matching files returns exactly that.
func TestFilewalk_NoMatchingFiles(t *testing.T) {
	t.Parallel()

	fwt := newFilewalkerTest(t)
	fwt.fileNameRegex = regexp.MustCompile(`^\.env$`)

	results := fwt.run(t, fstest.MapFS{
		"home/kiwi/notes.txt": {},
	})

	require.Empty(t, results)
}

// All top-level globbed entries should qualify for walking.
func TestFilewalk_GlobMatchesAllRootDirs(t *testing.T) {
	t.Parallel()

	fwt := newFilewalkerTest(t)
	fwt.rootDirs = []string{"home/*"}
	fwt.fileType = fileTypeFile

	results := fwt.run(t, fstest.MapFS{
		"home/kiwi/.env":  {},
		"home/mango/.env": {},
	})

	require.ElementsMatch(t, []string{"home/kiwi/.env", "home/mango/.env"}, results)
}

// A top-level globbed file that matches should be reported.
func TestFilewalk_GlobMatchesFile(t *testing.T) {
	t.Parallel()

	fwt := newFilewalkerTest(t)
	fwt.rootDirs = []string{"home/*"}
	fwt.fileType = fileTypeFile

	results := fwt.run(t, fstest.MapFS{
		"home/notes.txt": {},
		"home/kiwi/.env": {},
	})

	require.ElementsMatch(t, []string{"home/notes.txt", "home/kiwi/.env"}, results)
}

// SkipDirs should not apply to files.
func TestFilewalk_SkipDirsIgnoresFiles(t *testing.T) {
	t.Parallel()

	fwt := newFilewalkerTest(t)
	fwt.fileType = fileTypeFile
	fwt.skipDirs = []*regexp.Regexp{regexp.MustCompile(`node_modules$`)}

	results := fwt.run(t, fstest.MapFS{
		"home/kiwi/node_modules": {},
		"home/kiwi/other.env":    {},
	})

	require.ElementsMatch(t, []string{"home/kiwi/node_modules", "home/kiwi/other.env"}, results)
}

// SkipDir paths which also match other filters should not be reported.
func TestFilewalk_SkippedDirNotReported(t *testing.T) {
	t.Parallel()

	fwt := newFilewalkerTest(t)
	fwt.fileType = fileTypeDir
	fwt.fileNameRegex = regexp.MustCompile(`^(node_modules|src)$`)
	fwt.skipDirs = []*regexp.Regexp{regexp.MustCompile(`node_modules$`)}

	results := fwt.run(t, fstest.MapFS{
		"home/kiwi/src/main.go":         {},
		"home/kiwi/node_modules/pkg.js": {},
	})

	require.ElementsMatch(t, []string{"home/kiwi/src"}, results)
}

// SkipDirs run against a full path.
func TestFilewalk_SkipDirsMatchFullPath(t *testing.T) {
	t.Parallel()

	fwt := newFilewalkerTest(t)
	fwt.fileNameRegex = regexp.MustCompile(`^aws-credentials$`)
	fwt.skipDirs = []*regexp.Regexp{regexp.MustCompile(`specificproject[\\/]secrets$`)}

	results := fwt.run(t, fstest.MapFS{
		"home/kiwi/proj/aws-credentials":                         {},
		"home/kiwi/specificproject/secrets/prod/aws-credentials": {},
	})

	require.ElementsMatch(t, []string{"home/kiwi/proj/aws-credentials"}, results)
}

// Configs sharing a root should split at directories only some of them skip.
func TestFilewalk_SharedRootSplitsAtSkipDirs(t *testing.T) {
	t.Parallel()

	results := walkMany(t, fstest.MapFS{
		"proj/.env":                       {},
		"proj/.git/HEAD":                  {},
		"proj/.git/hooks/.env":            {},
		"proj/node_modules/pkg/.env":      {},
		"proj/node_modules/pkg/.git/HEAD": {},
	}, map[string]filewalkConfig{
		"dotenv": testConfig(t, ".", `^\.env$`, fileTypeFile, `node_modules$`, `\.git$`),
		"git":    testConfig(t, ".", `^\.git$`, fileTypeDir, `node_modules$`, `\.git[\\/]`),
	})

	require.ElementsMatch(t, []string{"proj/.env"}, results["dotenv"])
	require.ElementsMatch(t, []string{"proj/.git"}, results["git"])
}

// A config rooted inside another's root should only match beneath its own root.
func TestFilewalk_NestedRootActivatesPartwayDown(t *testing.T) {
	t.Parallel()

	results := walkMany(t, fstest.MapFS{
		"home/kiwi/.env":              {},
		"home/kiwi/projects/api/.env": {},
	}, map[string]filewalkConfig{
		"everywhere": testConfig(t, ".", `^\.env$`, fileTypeFile),
		"projects":   testConfig(t, "home/kiwi/projects", `^\.env$`, fileTypeFile),
	})

	require.ElementsMatch(t, []string{"home/kiwi/.env", "home/kiwi/projects/api/.env"}, results["everywhere"])
	require.ElementsMatch(t, []string{"home/kiwi/projects/api/.env"}, results["projects"])
}

// The walk should keep descending through a dir every active config skips when another config's root lies below it.
func TestFilewalk_NestedRootUnderSkippedDir(t *testing.T) {
	t.Parallel()

	results := walkMany(t, fstest.MapFS{
		"home/kiwi/.env":              {},
		"home/kiwi/projects/api/.env": {},
		"home/ostrich/.env":           {},
	}, map[string]filewalkConfig{
		"skips_kiwi": testConfig(t, ".", `^\.env$`, fileTypeFile, `kiwi$`),
		"projects":   testConfig(t, "home/kiwi/projects", `^\.env$`, fileTypeFile),
	})

	require.ElementsMatch(t, []string{"home/ostrich/.env"}, results["skips_kiwi"])
	require.ElementsMatch(t, []string{"home/kiwi/projects/api/.env"}, results["projects"])
}

// A cancelled walk should leave previously stored results in place.
func TestFilewalk_CancelledKeepsPreviousResults(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.CopyFS(root, fstest.MapFS{"home/kiwi/.env": {}}))
	slogger := multislogger.NewNopLogger()
	store, err := storageci.NewStore(t, slogger, storage.FilewalkResultsStore.String())
	require.NoError(t, err)
	require.NoError(t, store.Set([]byte("dotenv"), []byte(`["previous"]`)))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	cfg := testConfig(t, root, `^\.env$`, fileTypeFile)
	newTestFilewalker(t, "dotenv", cfg, store, slogger).Filewalk(ctx)

	raw, err := store.Get([]byte("dotenv"))
	require.NoError(t, err)
	require.JSONEq(t, `["previous"]`, string(raw))
	lastWalk, err := store.Get(LastWalkTimeKey("dotenv"))
	require.NoError(t, err)
	require.Nil(t, lastWalk)
}

// testConfig builds a single-root config; rootDir is slash-separated and may be relative to the walked tree.
func testConfig(t *testing.T, rootDir, fileNameRegex, fileType string, skipDirs ...string) filewalkConfig {
	t.Helper()
	ft := &fileTypeFilter{}
	require.NoError(t, json.Unmarshal([]byte(`"`+fileType+`"`), ft))
	skips := make([]*regexp.Regexp, 0, len(skipDirs))
	for _, skipDir := range skipDirs {
		skips = append(skips, regexp.MustCompile(skipDir))
	}
	rootDirs := []string{filepath.FromSlash(rootDir)}
	return filewalkConfig{
		filewalkDefinition: filewalkDefinition{
			RootDirs:       &rootDirs,
			FileNameRegex:  regexp.MustCompile(fileNameRegex),
			SkipDirs:       &skips,
			FileTypeFilter: ft,
		},
	}
}

// walkMany walks every config in a single walk over fsys, returning each config's results relative to the tree.
func walkMany(t *testing.T, fsys fstest.MapFS, cfgs map[string]filewalkConfig) map[string][]string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.CopyFS(root, fsys))

	for name, cfg := range cfgs {
		rootDirs := make([]string, 0, len(*cfg.RootDirs))
		for _, rootDir := range *cfg.RootDirs {
			rootDirs = append(rootDirs, filepath.Join(root, rootDir))
		}
		cfg.RootDirs = &rootDirs
		cfgs[name] = cfg
	}

	slogger := multislogger.NewNopLogger()
	store, err := storageci.NewStore(t, slogger, storage.FilewalkResultsStore.String())
	require.NoError(t, err)
	newFilewalker(resolve(t.Context(), slogger, cfgs, runtime.GOOS), store, slogger).Filewalk(t.Context())

	results := make(map[string][]string, len(cfgs))
	for name := range cfgs {
		raw, err := store.Get([]byte(name))
		require.NoError(t, err)
		var paths []string
		require.NoError(t, json.Unmarshal(raw, &paths))
		results[name] = relPaths(t, root, paths)
	}
	return results
}

// Wraps configuring a filewalker in a temp directory and grabbing its results.
type filewalkerTest struct {
	name    string
	rootDir string

	rootDirs      []string
	fileType      string
	fileNameRegex *regexp.Regexp
	skipDirs      []*regexp.Regexp
}

func newFilewalkerTest(t *testing.T) *filewalkerTest {
	return &filewalkerTest{
		name:     t.Name(),
		rootDir:  t.TempDir(),
		rootDirs: []string{"."},
	}
}

// Runs a file walk with the provided configuration, returning the results stored.
func (fwt *filewalkerTest) run(t *testing.T, fsys fstest.MapFS) []string {
	require.NoError(t, os.CopyFS(fwt.rootDir, fsys))

	rootDirs := make([]string, 0, len(fwt.rootDirs))
	for _, rootDir := range fwt.rootDirs {
		rootDirs = append(rootDirs, filepath.Join(fwt.rootDir, filepath.FromSlash(rootDir)))
	}

	cfg := filewalkConfig{
		WalkInterval: duration(1 * time.Minute),
		filewalkDefinition: filewalkDefinition{
			RootDirs:      &rootDirs,
			FileNameRegex: fwt.fileNameRegex,
			SkipDirs:      &fwt.skipDirs,
		},
	}
	if fwt.fileType != "" {
		rawFileType, err := json.Marshal(fwt.fileType)
		require.NoError(t, err)
		cfg.FileTypeFilter = &fileTypeFilter{}
		require.NoError(t, json.Unmarshal(rawFileType, cfg.FileTypeFilter))
	}

	slogger := multislogger.NewNopLogger()
	store, err := storageci.NewStore(t, slogger, storage.FilewalkResultsStore.String())
	require.NoError(t, err)
	newTestFilewalker(t, fwt.name, cfg, store, slogger).Filewalk(t.Context())

	rawResults, err := store.Get([]byte(fwt.name))
	require.NoError(t, err)
	var walked []string
	require.NoError(t, json.Unmarshal(rawResults, &walked))

	results := make([]string, 0, len(walked))
	for _, path := range walked {
		relPath, err := filepath.Rel(fwt.rootDir, path)
		require.NoError(t, err)
		results = append(results, filepath.ToSlash(relPath))
	}
	return results
}

func newTestFilewalker(tb testing.TB, name string, cfg filewalkConfig, store types.GetterSetterDeleter, slogger *slog.Logger) *filewalker {
	return newFilewalker(resolve(tb.Context(), slogger, map[string]filewalkConfig{name: cfg}, runtime.GOOS), store, slogger)
}

func BenchmarkFilewalk(b *testing.B) {
	// Pick a directory guaranteed to exist on GH runners
	var testDir string
	switch runtime.GOOS {
	case "windows":
		testDir = `D:\a\`
	case "darwin":
		testDir = "/Users/"
	default:
		testDir = "/home/"
	}

	store, err := storageci.NewStore(b, multislogger.NewNopLogger(), storage.FilewalkResultsStore.String())
	require.NoError(b, err)

	testFilewalker := newTestFilewalker(b, "benchtest", filewalkConfig{
		WalkInterval: duration(1 * time.Minute),
		filewalkDefinition: filewalkDefinition{
			RootDirs:      &[]string{testDir},
			FileNameRegex: nil,
		},
	}, store, multislogger.NewNopLogger())

	b.ReportAllocs()
	for b.Loop() {
		testFilewalker.Filewalk(b.Context())
	}
}
