package tufci

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/kolide/kit/fsutil"
	"github.com/kolide/launcher/pkg/packaging"
	"github.com/stretchr/testify/require"
)

var testOsqueryBinary string

// downloadOnceFunc downloads a real osquery binary for use in tests. This function
// can be called multiple times but will only execute once -- the osquery binary is
// stored at path `testOsqueryBinary` and can be reused by all subsequent tests.
var downloadOnceFunc = sync.OnceFunc(func() {
	downloadDir, err := os.MkdirTemp("", "tufci")
	if err != nil {
		fmt.Printf("failed to make temp dir for test osquery binary: %v", err)
		os.Exit(1) //nolint:forbidigo // Fine to use os.Exit inside tests
	}

	target := packaging.Target{}
	if err := target.PlatformFromString(runtime.GOOS); err != nil {
		fmt.Printf("error parsing platform %s: %v", runtime.GOOS, err)
		os.RemoveAll(downloadDir) // explicit removal as defer will not run when os.Exit is called
		os.Exit(1)                //nolint:forbidigo // Fine to use os.Exit inside tests
	}
	target.Arch = packaging.ArchFlavor(runtime.GOARCH)
	if runtime.GOOS == "darwin" {
		target.Arch = packaging.Universal
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dlPath, err := packaging.FetchBinary(ctx, downloadDir, "osqueryd", target.PlatformBinaryName("osqueryd"), "nightly", target)
	if err != nil {
		fmt.Printf("error fetching binary osqueryd binary: %v", err)
		cancel()                  // explicit cancel as defer will not run when os.Exit is called
		os.RemoveAll(downloadDir) // explicit removal as defer will not run when os.Exit is called
		os.Exit(1)                //nolint:forbidigo // Fine to use os.Exit inside tests
	}

	testOsqueryBinary = filepath.Join(downloadDir, filepath.Base(dlPath))
	if runtime.GOOS == "windows" {
		testOsqueryBinary += ".exe"
	}

	if err := fsutil.CopyFile(dlPath, testOsqueryBinary); err != nil {
		fmt.Printf("error copying osqueryd binary: %v", err)
		cancel()                  // explicit cancel as defer will not run when os.Exit is called
		os.RemoveAll(downloadDir) // explicit removal as defer will not run when os.Exit is called
		os.Exit(1)                //nolint:forbidigo // Fine to use os.Exit inside tests
	}
})

// CopyBinary ensures we've downloaded a test osquery binary, then creates a symlink
// between the real binary and the expected `executablePath` location. (We used to
// actually copy the entire binary, but found that was very slow.)
func CopyBinary(t *testing.T, executablePath string) {
	downloadOnceFunc()

	require.NoError(t, os.MkdirAll(filepath.Dir(executablePath), 0755))
	require.NoError(t, os.Symlink(testOsqueryBinary, executablePath))
}

var testOlderOsqueryBinary string

const OlderBinaryVersion = "5.17.0"

var downloadOlderOnceFunc = sync.OnceFunc(func() {
	downloadDir, err := os.MkdirTemp("", "tufci")
	if err != nil {
		fmt.Printf("failed to make temp dir for test osquery binary: %v", err)
		os.Exit(1) //nolint:forbidigo // Fine to use os.Exit inside tests
	}

	target := packaging.Target{}
	if err := target.PlatformFromString(runtime.GOOS); err != nil {
		fmt.Printf("error parsing platform %s: %v", runtime.GOOS, err)
		os.RemoveAll(downloadDir) // explicit removal as defer will not run when os.Exit is called
		os.Exit(1)                //nolint:forbidigo // Fine to use os.Exit inside tests
	}
	target.Arch = packaging.ArchFlavor(runtime.GOARCH)
	if runtime.GOOS == "darwin" {
		target.Arch = packaging.Universal
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Hardcode an older version of osquery
	dlPath, err := packaging.FetchBinary(ctx, downloadDir, "osqueryd", target.PlatformBinaryName("osqueryd"), OlderBinaryVersion, target)
	if err != nil {
		fmt.Printf("error fetching binary osqueryd binary: %v", err)
		cancel()                  // explicit cancel as defer will not run when os.Exit is called
		os.RemoveAll(downloadDir) // explicit removal as defer will not run when os.Exit is called
		os.Exit(1)                //nolint:forbidigo // Fine to use os.Exit inside tests
	}

	testOlderOsqueryBinary = filepath.Join(downloadDir, filepath.Base(dlPath))
	if runtime.GOOS == "windows" {
		testOlderOsqueryBinary += ".exe"
	}

	if err := fsutil.CopyFile(dlPath, testOlderOsqueryBinary); err != nil {
		fmt.Printf("error copying osqueryd binary: %v", err)
		cancel()                  // explicit cancel as defer will not run when os.Exit is called
		os.RemoveAll(downloadDir) // explicit removal as defer will not run when os.Exit is called
		os.Exit(1)                //nolint:forbidigo // Fine to use os.Exit inside tests
	}
})

func CopyOlderBinary(t *testing.T, executablePath string) {
	downloadOlderOnceFunc()

	require.NoError(t, os.MkdirAll(filepath.Dir(executablePath), 0755))
	require.NoError(t, os.Symlink(testOlderOsqueryBinary, executablePath))
}
