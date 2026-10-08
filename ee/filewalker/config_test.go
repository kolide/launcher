package filewalker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_duration_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName     string
		rawDuration      []byte
		expectedDuration time.Duration
		expectedError    bool
	}{
		{
			testCaseName:     "hours",
			rawDuration:      []byte(`"1h"`),
			expectedDuration: 1 * time.Hour,
		},
		{
			testCaseName:     "minutes",
			rawDuration:      []byte(`"30m"`),
			expectedDuration: 30 * time.Minute,
		},
		{
			testCaseName:     "compound",
			rawDuration:      []byte(`"1h30m"`),
			expectedDuration: 90 * time.Minute,
		},
		{
			testCaseName:  "invalid",
			rawDuration:   []byte(`"notaduration"`),
			expectedError: true,
		},
	} {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			var d duration
			err := json.Unmarshal(tt.rawDuration, &d)
			if tt.expectedError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expectedDuration, time.Duration(d))
		})
	}
}

func Test_fileTypeFilter_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName         string
		fileTypeRaw          []byte
		expectedFileTypeName string
		expectedError        bool
	}{
		{
			testCaseName:         fileTypeFile,
			fileTypeRaw:          []byte(`"file"`),
			expectedFileTypeName: fileTypeFile,
			expectedError:        false,
		},
		{
			testCaseName:         fileTypeDir,
			fileTypeRaw:          []byte(`"dir"`),
			expectedFileTypeName: fileTypeDir,
			expectedError:        false,
		},
		{
			testCaseName:         "invalid",
			fileTypeRaw:          []byte(`"notsupported"`),
			expectedFileTypeName: "",
			expectedError:        true,
		},
	} {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			var ft fileTypeFilter
			err := json.Unmarshal(tt.fileTypeRaw, &ft)
			if tt.expectedError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expectedFileTypeName, ft.name)
		})
	}
}

func Test_fileTypeFilter_matches(t *testing.T) {
	t.Parallel()

	// Set up directory to test against
	tempDir := t.TempDir()
	dirInfo, err := os.Stat(tempDir)
	require.NoError(t, err)

	// Set up file to test against
	tempFile := filepath.Join(tempDir, uuid.NewString())
	require.NoError(t, os.WriteFile(tempFile, []byte("test"), 0755))
	fileInfo, err := os.Stat(tempFile)
	require.NoError(t, err)

	// Test fileTypeFile first
	var ftFile fileTypeFilter
	require.NoError(t, json.Unmarshal([]byte(`"file"`), &ftFile))

	require.False(t, ftFile.matches(dirInfo.Mode()))
	require.True(t, ftFile.matches(fileInfo.Mode()))

	// Test fileTypeDir next
	var ftDir fileTypeFilter
	require.NoError(t, json.Unmarshal([]byte(`"dir"`), &ftDir))

	require.True(t, ftDir.matches(dirInfo.Mode()))
	require.False(t, ftDir.matches(fileInfo.Mode()))
}

func TestResolve_Overlays(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.CopyFS(root, fstest.MapFS{
		"test-1/x":     {},
		"test-2/x":     {},
		"test-3/x":     {},
		"test-4/x":     {},
		"test-5/x":     {},
		"test-other/x": {},
		"home/kiwi/x":  {},
		"users/kiwi/x": {},
	}))
	rootDirs := func(rel string) *[]string {
		return &[]string{filepath.Join(root, filepath.FromSlash(rel))}
	}

	testRegex := regexp.MustCompile(".*")
	testSkipDir := regexp.MustCompile(`\/tmp\/\.git`)
	fileType := &fileTypeFilter{}
	require.NoError(t, json.Unmarshal([]byte(`"file"`), fileType))

	for _, tt := range []struct {
		testCaseName    string
		cfg             filewalkConfig
		expectedMatcher matcher
		expectedRoots   []string
	}{
		{
			testCaseName: "no overlays, no filename regex, no skip dirs",
			cfg: filewalkConfig{
				filewalkDefinition: filewalkDefinition{
					RootDirs: rootDirs("test-1"),
				},
			},
			expectedRoots: []string{"test-1"},
		},
		{
			testCaseName: "no overlays, filename regex, skip dirs",
			cfg: filewalkConfig{
				filewalkDefinition: filewalkDefinition{
					RootDirs:      rootDirs("test-2"),
					FileNameRegex: testRegex,
					SkipDirs:      &[]*regexp.Regexp{testSkipDir},
				},
			},
			expectedMatcher: matcher{fileName: testRegex, skipDirs: []*regexp.Regexp{testSkipDir}},
			expectedRoots:   []string{"test-2"},
		},
		{
			testCaseName: "overlay exists but doesn't apply",
			cfg: filewalkConfig{
				filewalkDefinition: filewalkDefinition{
					RootDirs: rootDirs("test-3"),
				},
				Overlays: []filewalkConfigOverlay{
					{
						Filters: map[string]string{
							"goos": "darwin",
						},
						filewalkDefinition: filewalkDefinition{
							RootDirs:      rootDirs("test-other"),
							FileNameRegex: testRegex,
							SkipDirs:      &[]*regexp.Regexp{testSkipDir},
						},
					},
				},
			},
			expectedRoots: []string{"test-3"},
		},
		{
			testCaseName: "overlay, still no filename regex",
			cfg: filewalkConfig{
				Overlays: []filewalkConfigOverlay{
					{
						Filters: map[string]string{
							"goos": "linux",
						},
						filewalkDefinition: filewalkDefinition{
							RootDirs: rootDirs("test-4"),
						},
					},
				},
			},
			expectedRoots: []string{"test-4"},
		},
		{
			testCaseName: "overlay, filename regex, skipdirs",
			cfg: filewalkConfig{
				Overlays: []filewalkConfigOverlay{
					{
						Filters: map[string]string{
							"goos": "linux",
						},
						filewalkDefinition: filewalkDefinition{
							RootDirs:      rootDirs("test-5"),
							FileNameRegex: testRegex,
							SkipDirs:      &[]*regexp.Regexp{testSkipDir},
						},
					},
				},
			},
			expectedMatcher: matcher{fileName: testRegex, skipDirs: []*regexp.Regexp{testSkipDir}},
			expectedRoots:   []string{"test-5"},
		},
		{
			testCaseName: "overlay only overrides what it sets",
			cfg: filewalkConfig{
				filewalkDefinition: filewalkDefinition{
					FileNameRegex:  testRegex,
					SkipDirs:       &[]*regexp.Regexp{testSkipDir},
					FileTypeFilter: fileType,
				},
				Overlays: []filewalkConfigOverlay{
					{
						Filters: map[string]string{
							"goos": "darwin",
						},
						filewalkDefinition: filewalkDefinition{
							RootDirs: rootDirs("users/*"),
						},
					},
					{
						Filters: map[string]string{
							"goos": "linux",
						},
						filewalkDefinition: filewalkDefinition{
							RootDirs: rootDirs("home/*"),
						},
					},
				},
			},
			expectedMatcher: matcher{fileName: testRegex, skipDirs: []*regexp.Regexp{testSkipDir}, fileType: fileType},
			expectedRoots:   []string{"home/kiwi"},
		},
	} {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			spec := resolve(t.Context(), multislogger.NewNopLogger(), map[string]filewalkConfig{"test_filewalk": tt.cfg}, "linux")

			tt.expectedMatcher.name = "test_filewalk"
			require.Len(t, spec.matchers, 1)
			require.Equal(t, tt.expectedMatcher, *spec.matchers[0])
			require.ElementsMatch(t, tt.expectedRoots, relPaths(t, root, spec.roots))
		})
	}
}

func TestResolve_Roots(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.CopyFS(root, fstest.MapFS{
		"etc/ssh/ssh_host_ed25519_key":    {},
		"home/kiwi/projects/api/.env":     {},
		"home/kiwi.bak/projects/old/.env": {},
		"home/ostrich/.netrc":             {},
	}))
	rootDirs := func(rel string) *[]string {
		return &[]string{filepath.Join(root, filepath.FromSlash(rel))}
	}

	spec := resolve(t.Context(), multislogger.NewNopLogger(), map[string]filewalkConfig{
		"dotenv_files":  {filewalkDefinition: filewalkDefinition{RootDirs: rootDirs("home/*")}},
		"kiwi_projects": {filewalkDefinition: filewalkDefinition{RootDirs: rootDirs("home/kiwi/projects")}},
		"host_keys":     {filewalkDefinition: filewalkDefinition{RootDirs: rootDirs("etc/ssh")}},
		"opt_apps":      {filewalkDefinition: filewalkDefinition{RootDirs: rootDirs("opt/*")}},
	}, "linux")

	names := make([]string, 0, len(spec.matchers))
	for _, m := range spec.matchers {
		names = append(names, m.name)
	}
	require.ElementsMatch(t, []string{"dotenv_files", "kiwi_projects", "host_keys", "opt_apps"}, names)

	require.ElementsMatch(t, []string{"etc/ssh", "home/kiwi", "home/kiwi.bak", "home/ostrich"}, relPaths(t, root, spec.roots))

	activated := make(map[string][]string)
	for key, matchers := range spec.activate {
		rel := relPaths(t, canonicalize(root), []string{key})[0]
		for _, m := range matchers {
			activated[rel] = append(activated[rel], m.name)
		}
	}
	require.Equal(t, map[string][]string{
		"etc/ssh":            {"host_keys"},
		"home/kiwi":          {"dotenv_files"},
		"home/kiwi.bak":      {"dotenv_files"},
		"home/ostrich":       {"dotenv_files"},
		"home/kiwi/projects": {"kiwi_projects"},
	}, activated)
}

func relPaths(t *testing.T, base string, paths []string) []string {
	t.Helper()
	rels := make([]string, 0, len(paths))
	for _, path := range paths {
		rel, err := filepath.Rel(base, path)
		require.NoError(t, err)
		rels = append(rels, filepath.ToSlash(rel))
	}
	return rels
}
