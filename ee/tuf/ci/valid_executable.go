package tufci

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/kolide/launcher/pkg/osquery/testutil"
	"github.com/stretchr/testify/require"
)

var (
	testOsqueryBinary   string
	downloadOsqueryOnce sync.Once
	downloadOsqueryErr  error
)

// CopyBinary ensures we've downloaded a test osquery binary, then creates a symlink
// to it at the expected `executablePath` location. The cached binary is already signed,
// so the symlink will point to an executable binary.
func CopyBinary(t *testing.T, executablePath string) {
	// Download osquery binary once, but fail the test if download fails
	downloadOsqueryOnce.Do(func() {
		testOsqueryBinary, _, downloadOsqueryErr = testutil.DownloadOsquery("nightly")
	})
	if downloadOsqueryErr != nil {
		t.Fatalf("failed to download osquery binary: %v", downloadOsqueryErr)
	}

	require.NoError(t, os.MkdirAll(filepath.Dir(executablePath), 0755))
	require.NoError(t, os.Symlink(testOsqueryBinary, executablePath))
}

var (
	testOlderOsqueryBinary   string
	downloadOlderOsqueryOnce sync.Once
	downloadOlderOsqueryErr  error
)

const OlderBinaryVersion = "5.17.0"

func CopyOlderBinary(t *testing.T, executablePath string) {
	// Download older osquery binary once, but fail the test if download fails
	downloadOlderOsqueryOnce.Do(func() {
		testOlderOsqueryBinary, _, downloadOlderOsqueryErr = testutil.DownloadOsquery(OlderBinaryVersion)
	})
	if downloadOlderOsqueryErr != nil {
		t.Fatalf("failed to download older osquery binary: %v", downloadOlderOsqueryErr)
	}

	require.NoError(t, os.MkdirAll(filepath.Dir(executablePath), 0755))
	require.NoError(t, os.Symlink(testOlderOsqueryBinary, executablePath))
}
