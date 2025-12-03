package tufci

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/kolide/launcher/pkg/osquery/testutil"
	"github.com/stretchr/testify/require"
)

var testOsqueryBinary string

// downloadOnceFunc downloads a real osquery binary for use in tests. This function
// can be called multiple times but will only execute once -- the osquery binary is
// stored at path `testOsqueryBinary` and can be reused by all subsequent tests.
var downloadOnceFunc = sync.OnceFunc(func() {
	testOsqueryBinary, _, _ = testutil.DownloadOsquery("nightly")
})

// CopyBinary ensures we've downloaded a test osquery binary, then creates a symlink
// to it at the expected `executablePath` location. The cached binary is already signed,
// so the symlink will point to an executable binary.
func CopyBinary(t *testing.T, executablePath string) {
	downloadOnceFunc()

	require.NoError(t, os.MkdirAll(filepath.Dir(executablePath), 0755))
	require.NoError(t, os.Symlink(testOsqueryBinary, executablePath))
}

var testOlderOsqueryBinary string

const OlderBinaryVersion = "5.17.0"

var downloadOlderOnceFunc = sync.OnceFunc(func() {
	testOlderOsqueryBinary, _, _ = testutil.DownloadOsquery(OlderBinaryVersion)
})

func CopyOlderBinary(t *testing.T, executablePath string) {
	downloadOlderOnceFunc()

	require.NoError(t, os.MkdirAll(filepath.Dir(executablePath), 0755))
	require.NoError(t, os.Symlink(testOlderOsqueryBinary, executablePath))
}
