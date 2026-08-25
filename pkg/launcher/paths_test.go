package launcher

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/stretchr/testify/require"
)

const kolideServerURLForTest = "k2device.kolide.com"

func TestDetermineRootDirectoryOverride(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName string
		// setup preps and returns the provided dir and opts, plus the expected result
		setup func(t *testing.T) (rootDir string, opts rootDirOverrideOpts, expectedRootDir string)
	}{
		{
			testCaseName: "non-kolide server url passthrough",
			setup: func(t *testing.T) (string, rootDirOverrideOpts, string) {
				opts := testOverrideOpts(wellKnownDir(t, "launcher db contents"))
				opts.kolideServerURL = "https://example.com"

				rootDir := filepath.Join("some", "dir", "somewhere")
				return rootDir, opts, rootDir
			},
		},
		{
			testCaseName: "database already in passed directory passthrough",
			setup: func(t *testing.T) (string, rootDirOverrideOpts, string) {
				rootDir := t.TempDir()
				writeDB(t, rootDir, "launcher db contents")

				return rootDir, testOverrideOpts(wellKnownDir(t, "launcher db contents")), rootDir
			},
		},
		{
			testCaseName: "root directory cannot be checked passthrough",
			setup: func(t *testing.T) (string, rootDirOverrideOpts, string) {
				// the NUL byte makes os.Stat fail with EINVAL rather than a not-exist error
				rootDir := filepath.Join(t.TempDir(), "root\x00dir")

				return rootDir, testOverrideOpts(wellKnownDir(t, "launcher db contents")), rootDir
			},
		},
		{
			testCaseName: "all well-known empty databases passthrough",
			setup: func(t *testing.T) (string, rootDirOverrideOpts, string) {
				rootDir := filepath.Join(t.TempDir(), "does-not-exist")

				return rootDir, testOverrideOpts(wellKnownDir(t, "")), rootDir
			},
		},
		{
			testCaseName: "well-known databases absent passthrough",
			setup: func(t *testing.T) (string, rootDirOverrideOpts, string) {
				rootDir := filepath.Join(t.TempDir(), "does-not-exist")

				return rootDir, testOverrideOpts(wellKnownDirNoDB(t)), rootDir
			},
		},
		{
			testCaseName: "unprivileged passthrough",
			setup: func(t *testing.T) (string, rootDirOverrideOpts, string) {
				opts := testOverrideOpts(wellKnownDir(t, "launcher db contents"))
				opts.isPrivileged = false

				rootDir := filepath.Join(t.TempDir(), "does-not-exist")
				return rootDir, opts, rootDir
			},
		},
		{
			testCaseName: "well-known database works returns override",
			setup: func(t *testing.T) (string, rootDirOverrideOpts, string) {
				overrideDir := wellKnownDir(t, "launcher db contents")

				return filepath.Join(t.TempDir(), "does-not-exist"), testOverrideOpts(overrideDir), overrideDir
			},
		},
	} {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			rootDir, opts, expectedRootDir := tt.setup(t)

			require.Equal(
				t,
				expectedRootDir,
				rootDirectoryOverride(rootDir, opts),
				"incorrect root directory",
			)
		})
	}
}

// testOverrideOpts defaults to the privileged kolide-hosted case, so that each test case
// only has to state how it differs from one that would produce an override.
func testOverrideOpts(wellKnownRootDirs ...string) rootDirOverrideOpts {
	return rootDirOverrideOpts{
		logger:            multislogger.NewNopLogger(),
		kolideServerURL:   kolideServerURLForTest,
		isPrivileged:      true,
		wellKnownRootDirs: wellKnownRootDirs,
	}
}

// wellKnownDirNoDB returns a stand-in for one of the likelyWindowsRootDirPaths -- the
// package identifier must appear in the path for it to be considered a candidate.
func wellKnownDirNoDB(t *testing.T) string {
	dir := filepath.Join(t.TempDir(), "Kolide", "Launcher-"+DefaultLauncherIdentifier, "data")
	require.NoError(t, os.MkdirAll(dir, 0755))
	return dir
}

func wellKnownDir(t *testing.T, dbContents string) string {
	dir := wellKnownDirNoDB(t)
	writeDB(t, dir, dbContents)
	return dir
}

func writeDB(t *testing.T, dir string, contents string) {
	require.NoError(t, os.WriteFile(filepath.Join(dir, "launcher.db"), []byte(contents), 0644))
}
