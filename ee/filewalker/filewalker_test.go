package filewalker

import (
	"regexp"
	"runtime"
	"testing"
	"time"

	"github.com/kolide/launcher/ee/agent/storage"
	storageci "github.com/kolide/launcher/ee/agent/storage/ci"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

func TestUpdateConfig(t *testing.T) {
	t.Parallel()

	testRegex := regexp.MustCompile(".*")

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
	}{
		{
			testCaseName: "no overlays, no filename regex",
			cfg: filewalkConfig{
				WalkInterval: 1 * time.Minute,
				filewalkDefinition: filewalkDefinition{
					RootDirs:      &[]string{"test-1"},
					FileNameRegex: nil,
				},
			},
			expectedWalkInterval:  1 * time.Minute,
			expectedRootDirs:      []string{"test-1"},
			expectedFileNameRegex: nil,
		},
		{
			testCaseName: "no overlays, filename regex",
			cfg: filewalkConfig{
				WalkInterval: 2 * time.Minute,
				filewalkDefinition: filewalkDefinition{
					RootDirs:      &[]string{"test-2"},
					FileNameRegex: testRegex,
				},
			},
			expectedWalkInterval:  2 * time.Minute,
			expectedRootDirs:      []string{"test-2"},
			expectedFileNameRegex: testRegex,
		},
		{
			testCaseName: "overlay exists but doesn't apply",
			cfg: filewalkConfig{
				WalkInterval: 3 * time.Minute,
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
						},
					},
				},
			},
			expectedWalkInterval:  3 * time.Minute,
			expectedRootDirs:      []string{"test-3"},
			expectedFileNameRegex: nil,
		},
		{
			testCaseName: "overlay, still no filename regex",
			cfg: filewalkConfig{
				WalkInterval: 4 * time.Minute,
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
		},
		{
			testCaseName: "overlay, filename regex",
			cfg: filewalkConfig{
				WalkInterval: 5 * time.Minute,
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
						},
					},
				},
			},
			expectedWalkInterval:  5 * time.Minute,
			expectedRootDirs:      []string{"test-5"},
			expectedFileNameRegex: testRegex,
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
	}
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
		WalkInterval: 1 * time.Minute,
		filewalkDefinition: filewalkDefinition{
			RootDirs:      &[]string{testDir},
			FileNameRegex: nil,
		},
	}, store, multislogger.NewNopLogger())

	b.ReportAllocs()
	for b.Loop() {
		testFilewalker.filewalk(b.Context())
	}
}
