package filewalker

import (
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
