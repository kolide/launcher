package filewalker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
	"testing/fstest"
	"time"

	"github.com/kolide/launcher/v2/ee/agent/storage"
	storageci "github.com/kolide/launcher/v2/ee/agent/storage/ci"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func TestUpdateConfig(t *testing.T) {
	t.Parallel()

	testRegex := regexp.MustCompile(".*")
	testSkipDir := regexp.MustCompile(`\/tmp\/\.git`)

	nonMatchingGoos := ""
	switch runtime.GOOS {
	case "darwin":
		nonMatchingGoos = "windows"
	case "windows":
		nonMatchingGoos = "linux"
	case "linux":
		nonMatchingGoos = "darwin"
	}

	for _, tt := range []struct {
		testCaseName          string
		cfg                   filewalkConfig
		expectedWalkInterval  time.Duration
		expectedRootDirs      []string
		expectedFileNameRegex *regexp.Regexp
		expectedSkipDirs      []*regexp.Regexp
	}{
		{
			testCaseName: "no overlays, no filename regex, no skip dirs",
			cfg: filewalkConfig{
				WalkInterval: duration(1 * time.Minute),
				filewalkDefinition: filewalkDefinition{
					RootDirs:      &[]string{"test-1"},
					FileNameRegex: nil,
				},
			},
			expectedWalkInterval:  1 * time.Minute,
			expectedRootDirs:      []string{"test-1"},
			expectedFileNameRegex: nil,
			expectedSkipDirs:      nil,
		},
		{
			testCaseName: "no overlays, filename regex, skip dirs",
			cfg: filewalkConfig{
				WalkInterval: duration(2 * time.Minute),
				filewalkDefinition: filewalkDefinition{
					RootDirs:      &[]string{"test-2"},
					FileNameRegex: testRegex,
					SkipDirs:      &[]*regexp.Regexp{testSkipDir},
				},
			},
			expectedWalkInterval:  2 * time.Minute,
			expectedRootDirs:      []string{"test-2"},
			expectedFileNameRegex: testRegex,
			expectedSkipDirs:      []*regexp.Regexp{testSkipDir},
		},
		{
			testCaseName: "overlay exists but doesn't apply",
			cfg: filewalkConfig{
				WalkInterval: duration(3 * time.Minute),
				filewalkDefinition: filewalkDefinition{
					RootDirs:      &[]string{"test-3"},
					FileNameRegex: nil,
				},
				Overlays: []filewalkConfigOverlay{
					{
						Filters: map[string]string{
							"goos": nonMatchingGoos,
						},
						filewalkDefinition: filewalkDefinition{
							RootDirs:      &[]string{"test-other"},
							FileNameRegex: testRegex,
							SkipDirs:      &[]*regexp.Regexp{testSkipDir},
						},
					},
				},
			},
			expectedWalkInterval:  3 * time.Minute,
			expectedRootDirs:      []string{"test-3"},
			expectedFileNameRegex: nil,
			expectedSkipDirs:      nil,
		},
		{
			testCaseName: "overlay, still no filename regex",
			cfg: filewalkConfig{
				WalkInterval: duration(4 * time.Minute),
				filewalkDefinition: filewalkDefinition{
					RootDirs:      nil,
					FileNameRegex: nil,
				},
				Overlays: []filewalkConfigOverlay{
					{
						Filters: map[string]string{
							"goos": runtime.GOOS,
						},
						filewalkDefinition: filewalkDefinition{
							RootDirs:      &[]string{"test-4"},
							FileNameRegex: nil,
						},
					},
				},
			},
			expectedWalkInterval:  4 * time.Minute,
			expectedRootDirs:      []string{"test-4"},
			expectedFileNameRegex: nil,
			expectedSkipDirs:      nil,
		},
		{
			testCaseName: "overlay, filename regex, skipdirs",
			cfg: filewalkConfig{
				WalkInterval: duration(5 * time.Minute),
				filewalkDefinition: filewalkDefinition{
					RootDirs:      nil,
					FileNameRegex: nil,
				},
				Overlays: []filewalkConfigOverlay{
					{
						Filters: map[string]string{
							"goos": runtime.GOOS,
						},
						filewalkDefinition: filewalkDefinition{
							RootDirs:      &[]string{"test-5"},
							FileNameRegex: testRegex,
							SkipDirs:      &[]*regexp.Regexp{testSkipDir},
						},
					},
				},
			},
			expectedWalkInterval:  5 * time.Minute,
			expectedRootDirs:      []string{"test-5"},
			expectedFileNameRegex: testRegex,
			expectedSkipDirs:      []*regexp.Regexp{testSkipDir},
		},
	} {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()
		})

		slogger := multislogger.NewNopLogger()
		resultsStore, err := storageci.NewStore(t, slogger, storage.FilewalkResultsStore.String())
		require.NoError(t, err)

		testFw := newFilewalker("test_filewalk_table", tt.cfg, resultsStore, slogger)
		require.Equal(t, tt.expectedWalkInterval, testFw.walkInterval)
		require.Equal(t, tt.expectedRootDirs, testFw.rootDirs)
		require.Equal(t, tt.expectedFileNameRegex, testFw.fileNameRegex)
		require.Equal(t, tt.expectedSkipDirs, testFw.skipDirs)
		require.Nil(t, testFw.fileTypeFilter)
	}
}

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

// Wraps configuring a filewalker in a temp directory and grabbing its results.
type filewalkerTest struct {
	name    string
	rootDir string

	fileType      string
	fileNameRegex *regexp.Regexp
	skipDirs      []*regexp.Regexp
}

func newFilewalkerTest(t *testing.T) *filewalkerTest {
	return &filewalkerTest{
		name:    t.Name(),
		rootDir: t.TempDir(),
	}
}

// Runs a file walk with the provided configuration, returning the results stored.
func (fwt *filewalkerTest) run(t *testing.T, fsys fstest.MapFS) []string {
	require.NoError(t, os.CopyFS(fwt.rootDir, fsys))

	cfg := filewalkConfig{
		WalkInterval: duration(1 * time.Minute),
		filewalkDefinition: filewalkDefinition{
			RootDirs:      &[]string{fwt.rootDir},
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
	newFilewalker(fwt.name, cfg, store, slogger).Filewalk(t.Context())

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

	testFilewalker := newFilewalker("benchtest", filewalkConfig{
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
