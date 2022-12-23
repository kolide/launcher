package autoupdate

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFindNewestSelf tests the FindNewestSelf. Hard to test this, as
// it's a light wrapper around os.Executable
func TestFindNewestSelf(t *testing.T) {
	t.Parallel()

	ctx := context.TODO()

	{
		newest, err := FindNewestSelf(ctx)
		require.NoError(t, err)
		require.Empty(t, newest, "No updates, should be empty")
	}

	// Let's try making a set of update directories
	binaryPath := os.Args[0]
	updatesDir := getUpdateDir(binaryPath)
	require.NotEmpty(t, updatesDir)
	defer os.RemoveAll(updatesDir)
	for _, n := range []string{"2", "5", "3", "1"} {
		require.NoError(t, os.MkdirAll(filepath.Join(updatesDir, n), 0755))
		f, err := os.Create(filepath.Join(updatesDir, n, "wrong-binary"))
		require.NoError(t, err)
		f.Close()
		require.NoError(t, os.Chmod(f.Name(), 0755))
	}

	{
		newest, err := FindNewestSelf(ctx)
		require.NoError(t, err)
		require.Empty(t, newest, "No correct binaries, should be empty")
	}

	for _, n := range []string{"2", "3"} {
		updatedBinaryPath := filepath.Join(updatesDir, n, filepath.Base(binaryPath))
		require.NoError(t, copyFile(updatedBinaryPath, binaryPath, false), "copy executable")
		require.NoError(t, os.Chmod(updatedBinaryPath, 0755), "chmod")
	}

	{
		expectedNewest := filepath.Join(updatesDir, "3", filepath.Base(binaryPath))
		newest, err := FindNewestSelf(ctx)
		require.NoError(t, err)
		require.Equal(t, expectedNewest, newest, "Should find newer binary")
	}

}

func TestGetUpdateDir(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		in  string
		out string
	}{
		{in: "/a/bin/path", out: "/a/bin/path-updates"},
		{in: "/a/bin/path-updates", out: "/a/bin/path-updates"},
		{in: "/a/bin/path-updates/1234/binary", out: "/a/bin/path-updates"},
		{in: "/a/bin/path/foo/bar-updates/1234/binary", out: "/a/bin/path/foo/bar-updates"},
		{in: "/a/bin/b-updates/123/b-updates/456/b", out: "/a/bin/b-updates"},
		{in: "/a/bin/path/", out: "/a/bin/path-updates"},
		{in: "/a/Test.app/Contents/MacOS/path", out: "/a/bin/path-updates"},
		{in: "/a/bin/path-updates/1234/Test.app/Contents/MacOS/path", out: "/a/bin/path-updates"},
		{in: "/a/bin/Test.app/Contents/MacOS/launcher-updates/1569339163/Test.app/Contents/MacOS/path", out: filepath.Clean("/a/bin/path-updates")},
		{in: "/a/bin/Test.app/Contents/MacOS/launcher", out: filepath.Clean("/a/bin/launcher-updates")},
		{in: "/a/bin/Test.app/Contents/MacOS/launcher-updates/1569339163/Test.app/Contents/MacOS/launcher", out: filepath.Clean("/a/bin/launcher-updates")},
		{in: "", out: ""},
		{in: "/", out: ""},
	}

	for _, tt := range tests {
		require.Equal(t, tt.out, getUpdateDir(tt.in), "input: %s", tt.in)
	}

}

func TestFindBaseDir(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		in  string
		out string
	}{
		{in: "", out: ""},
		{in: "/a/path/bin/launcher", out: filepath.Clean("/a/path/bin")},
		{in: "/a/path/bin/launcher-updates/1569339163/launcher", out: filepath.Clean("/a/path/bin")},
		{in: "/a/path/bin/launcher-updates/1569339163/Test.app/Contents/MacOS/launcher", out: filepath.Clean("/a/path/bin")},
		{in: "/a/path/Test.app/Contents/MacOS/launcher", out: filepath.Clean("/a/path/bin")},
		{in: "/a/path/Test.app/Contents/MacOS/launcher-updates/1569339163/Test.app/Contents/MacOS/launcher", out: filepath.Clean("/a/path/bin")},
		{in: "/a/path/bin/Test.app/Contents/MacOS/launcher", out: filepath.Clean("/a/path/bin")},
		{in: "/a/path/bin/Test.app/Contents/MacOS/launcher-updates/1569339163/Test.app/Contents/MacOS/launcher", out: filepath.Clean("/a/path/bin")},
	}

	for _, tt := range tests {
		require.Equal(t, tt.out, FindBaseDir(tt.in), "input: %s", tt.in)
	}

}

func TestFindNewestEmpty(t *testing.T) {
	t.Parallel()

	tmpDir, binaryName := setupTestDir(t, emptySetup)
	ctx := context.TODO()
	binaryPath := filepath.Join(tmpDir, binaryName)

	// Basic tests, test with binary and no updates
	require.Empty(t, FindNewest(ctx, ""), "passing empty string")
	require.Empty(t, FindNewest(ctx, tmpDir), "passing directory as arg")
	require.Equal(t, binaryPath, FindNewest(ctx, binaryPath), "no update directory")
}

func TestFindNewestEmptyUpdateDirs(t *testing.T) {
	t.Parallel()

	tmpDir, binaryName := setupTestDir(t, emptyUpdateDirs)
	ctx := context.TODO()
	binaryPath := filepath.Join(tmpDir, binaryName)

	require.Equal(t, binaryPath, FindNewest(ctx, binaryPath), "update dir, but no updates")
}

func TestFindNewestNonExecutable(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Windows doesn't use executable bit")
	}

	tmpDir, binaryName := setupTestDir(t, nonExecutableUpdates)
	ctx := context.TODO()
	binaryPath := filepath.Join(tmpDir, binaryName)
	updatesDir := fmt.Sprintf("%s%s", binaryPath, updateDirSuffix)

	require.Equal(t, binaryPath, FindNewest(ctx, binaryPath), "update dir, but only plain files")

	expectedNewest := filepath.Join(updatesDir, "1", "binary")
	require.NoError(t, os.Chmod(expectedNewest, 0755))
	if runtime.GOOS == "darwin" {
		expectedNewest = filepath.Join(updatesDir, "1", "Test.app", "Contents", "MacOS", "binary")
		require.NoError(t, os.Chmod(expectedNewest, 0755))
	}

	require.Equal(t,
		expectedNewest,
		FindNewest(ctx, binaryPath),
		"Should find number 1",
	)
}

func TestFindNewestExecutableUpdates(t *testing.T) {
	t.Parallel()

	tmpDir, binaryName := setupTestDir(t, executableUpdates)
	ctx := context.TODO()
	binaryPath := filepath.Join(tmpDir, binaryName)
	updatesDir := fmt.Sprintf("%s%s", binaryPath, updateDirSuffix)

	expectedNewest := filepath.Join(updatesDir, "5", "binary")
	if runtime.GOOS == "windows" {
		expectedNewest = expectedNewest + ".exe"
	} else if runtime.GOOS == "darwin" {
		expectedNewest = filepath.Join(updatesDir, "5", "Test.app", "Contents", "MacOS", "binary")
	}

	require.Equal(t, expectedNewest, FindNewest(ctx, binaryPath), "Should find number 5")
	require.Equal(t, expectedNewest, FindNewest(ctx, expectedNewest), "already running the newest")

}

func TestFindNewestCleanup(t *testing.T) {
	t.Parallel()

	// delete doesn't seem to work on windows. It gets a
	// "Access is denied" error". This may be a test setup
	// issue, or something with an open file handle.
	if runtime.GOOS == "windows" {
		t.Skip("TODO: Windows deletion test is broken")
	}

	tmpDir, binaryName := setupTestDir(t, executableUpdates)
	ctx := context.TODO()
	binaryPath := filepath.Join(tmpDir, binaryName)
	updatesDir := fmt.Sprintf("%s%s", binaryPath, updateDirSuffix)

	expectedNewest := filepath.Join(updatesDir, "5", "binary")
	if runtime.GOOS == "windows" {
		expectedNewest = expectedNewest + ".exe"
	} else if runtime.GOOS == "darwin" {
		expectedNewest = filepath.Join(updatesDir, "5", "Test.app", "Contents", "MacOS", "binary")
	}

	{
		updatesOnDisk, err := os.ReadDir(updatesDir)
		require.NoError(t, err)
		require.Equal(t, 4, len(updatesOnDisk))
		require.Equal(t, expectedNewest, FindNewest(ctx, binaryPath), "Should find number 5")
	}

	{
		_ = FindNewest(ctx, binaryPath, DeleteOldUpdates())
		updatesOnDisk, err := os.ReadDir(updatesDir)
		require.NoError(t, err)
		require.Equal(t, expectedNewest, FindNewest(ctx, binaryPath), "Should find number 5")
		require.Equal(t, 2, len(updatesOnDisk), "after delete")

	}
}

func TestCheckExecutableCorruptCleanup(t *testing.T) {
	t.Parallel()

	tmpDir, binaryName := setupTestDir(t, truncatedUpdates)
	ctx := context.TODO()
	binaryPath := filepath.Join(tmpDir, binaryName)
	updatesDir := fmt.Sprintf("%s%s", binaryPath, updateDirSuffix)

	expectedNewest := filepath.Join(updatesDir, "3", "binary")
	if runtime.GOOS == "windows" {
		expectedNewest = expectedNewest + ".exe"
	} else if runtime.GOOS == "darwin" {
		expectedNewest = filepath.Join(updatesDir, "3", "Test.app", "Contents", "MacOS", "binary")
	}

	{
		updatesOnDisk, err := os.ReadDir(updatesDir)
		require.NoError(t, err)
		require.Equal(t, 4, len(updatesOnDisk))
		require.Equal(t, expectedNewest, FindNewest(ctx, binaryPath), "Should find number 3")
	}

	{
		_ = FindNewest(ctx, binaryPath, DeleteCorruptUpdates())
		updatesOnDisk, err := os.ReadDir(updatesDir)
		require.NoError(t, err)
		require.Equal(t, 2, len(updatesOnDisk), "after cleaning corruption")
		require.Equal(t, expectedNewest, FindNewest(ctx, binaryPath), "Should find number 3")
	}
}

type setupState int

const (
	emptySetup setupState = iota
	emptyUpdateDirs
	nonExecutableUpdates
	executableUpdates
	truncatedUpdates
)

// setupTestDir function to setup the test dirs. This work is broken
// up in stages, allowing test functions to tap into various
// points. This is setup this way to allow simpler isolation on test
// failures.
func setupTestDir(t *testing.T, stage setupState) (string, string) {
	tmpDir := t.TempDir()

	// Create a test binary
	binaryName := windowsAddExe("binary")
	binaryPath := filepath.Join(tmpDir, binaryName)
	updatesDir := fmt.Sprintf("%s%s", binaryPath, updateDirSuffix)

	require.NoError(t, copyFile(binaryPath, os.Args[0], false), "copy executable")
	require.NoError(t, os.Chmod(binaryPath, 0755), "chmod")

	if stage <= emptySetup {
		return tmpDir, binaryName
	}

	// make some update directories
	// (these are out of order, to jumble up the create times)
	for _, n := range []string{"2", "5", "3", "1"} {
		require.NoError(t, os.MkdirAll(filepath.Join(updatesDir, n), 0755))
		if runtime.GOOS == "darwin" {
			require.NoError(t, os.MkdirAll(filepath.Join(updatesDir, n, "Test.app", "Contents", "MacOS"), 0755))
		}
	}

	if stage <= emptyUpdateDirs {
		return tmpDir, binaryName
	}

	// Copy executable to update directories
	for _, n := range []string{"2", "5", "3", "1"} {
		updatedBinaryPath := filepath.Join(updatesDir, n, binaryName)
		require.NoError(t, copyFile(updatedBinaryPath, binaryPath, false), "copy executable")
		if runtime.GOOS == "darwin" {
			updatedAppBundleBinaryPath := filepath.Join(updatesDir, n, "Test.app", "Contents", "MacOS", filepath.Base(binaryPath))
			require.NoError(t, copyFile(updatedAppBundleBinaryPath, binaryPath, false), "copy executable")
		}
	}

	if stage <= nonExecutableUpdates {
		return tmpDir, binaryName
	}

	// Make our top-level binaries executable
	for _, n := range []string{"2", "5", "3", "1"} {
		require.NoError(t, os.Chmod(filepath.Join(updatesDir, n, binaryName), 0755))
		if runtime.GOOS == "darwin" {
			require.NoError(t, os.Chmod(filepath.Join(updatesDir, n, "Test.app", "Contents", "MacOS", binaryName), 0755))
		}
	}

	if stage <= executableUpdates {
		return tmpDir, binaryName
	}

	for _, n := range []string{"5", "1"} {
		updatedBinaryPath := filepath.Join(updatesDir, n, binaryName)
		require.NoError(t, copyFile(updatedBinaryPath, binaryPath, true), "copy & truncate executable")
		if runtime.GOOS == "darwin" {
			require.NoError(t, copyFile(filepath.Join(updatesDir, n, "Test.app", "Contents", "MacOS", binaryName), binaryPath, true), "copy & truncate executable")
		}
	}

	return tmpDir, binaryName

}

// copyFile copies a file from srcPath to dstPath. If truncate is set,
// only half the file is copied. (This is a trivial wrapper to
// simplify setting up test cases)
func copyFile(dstPath, srcPath string, truncate bool) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	if !truncate {
		if _, err := io.Copy(dst, src); err != nil {
			return err
		}
	} else {
		stat, err := src.Stat()
		if err != nil {
			return fmt.Errorf("statting srcFile: %w", err)
		}

		if _, err = io.CopyN(dst, src, stat.Size()/2); err != nil {
			return fmt.Errorf("copying srcFile: %w", err)
		}
	}

	return nil
}

func TestCheckExecutable(t *testing.T) {
	t.Parallel()

	// We need to run this from a temp dir, else the early return
	// from matching os.Executable bypasses the point of this.
	tmpDir, binaryName := setupTestDir(t, executableUpdates)
	targetExe := filepath.Join(tmpDir, binaryName)

	var tests = []struct {
		testName    string
		expectedErr bool
	}{
		{
			testName:    "exit0",
			expectedErr: false,
		},
		{
			testName:    "exit1",
			expectedErr: false,
		},
		{
			testName:    "exit2",
			expectedErr: false,
		},
		{
			testName:    "sleep",
			expectedErr: true,
		},
	}

	for _, tt := range tests { // nolint:paralleltest
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			err := checkExecutable(context.TODO(), targetExe, "-test.run=TestHelperProcess", "--", tt.testName)
			if tt.expectedErr {
				require.Error(t, err, tt.testName)

				// As a bonus, we can test that if we call os.Args[0],
				// we still don't get an error. This is because we
				// trigger the match against os.Executable and don't
				// invoked. This is here, and not a dedicated test,
				// because we ensure the same test arguments.
				require.NoError(t, checkExecutable(context.TODO(), os.Args[0], "-test.run=TestHelperProcess", "--", tt.testName), "calling self with %s", tt.testName)
			} else {
				require.NoError(t, err, tt.testName)
			}
		})

	}
}

func TestCheckExecutableTruncated(t *testing.T) {
	t.Parallel()

	// First make a broken truncated binary. Lots of setup for this.
	truncatedBinary, err := os.CreateTemp("", "test-autoupdate-check-executable-truncation")
	require.NoError(t, err, "make temp file")
	defer os.Remove(truncatedBinary.Name())
	truncatedBinary.Close()

	require.NoError(t, copyFile(truncatedBinary.Name(), os.Args[0], true), "copy executable")
	require.NoError(t, os.Chmod(truncatedBinary.Name(), 0755))

	require.Error(t,
		checkExecutable(context.TODO(), truncatedBinary.Name(), "-test.run=TestHelperProcess", "--", "exit0"),
		"truncated binary")
}

func TestBuildTimestamp(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("FIXME: Windows")
	}

	var tests = []struct {
		buildTimestamp string
		expectedNewest string
		expectedOnDisk int
	}{
		{
			buildTimestamp: "0",
			expectedNewest: "5",
			expectedOnDisk: 2,
		},
		{
			buildTimestamp: "3",
			expectedNewest: "5",
			expectedOnDisk: 1, // remember, 4 is broken, so there should only be update 5 on disk
		},
		{
			buildTimestamp: "5",
			expectedOnDisk: 0,
		},
		{
			buildTimestamp: "6",
			expectedOnDisk: 0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run("buildTimestamp="+tt.buildTimestamp, func(t *testing.T) {
			t.Parallel()

			tmpDir, binaryName := setupTestDir(t, executableUpdates)
			ctx := context.TODO()

			binaryPath := filepath.Join(tmpDir, binaryName)
			updatesDir := binaryPath + updateDirSuffix

			returnedNewest := FindNewest(
				ctx,
				binaryPath,
				overrideBuildTimestamp(tt.buildTimestamp),
				DeleteOldUpdates(),
			)

			updatesOnDisk, err := os.ReadDir(updatesDir)
			require.NoError(t, err)
			require.Equal(t, tt.expectedOnDisk, len(updatesOnDisk), "remaining updates on disk")

			if tt.expectedNewest == "" {
				require.Equal(t, binaryPath, returnedNewest, "Expected to get original binary path")
			} else {
				updateFragment := strings.TrimPrefix(strings.TrimPrefix(returnedNewest, updatesDir), "/")
				expectedNewest := filepath.Join(tt.expectedNewest, "binary")
				if runtime.GOOS == "darwin" {
					expectedNewest = filepath.Join(tt.expectedNewest, "Test.app", "Contents", "MacOS", "binary")
				}
				require.Equal(t, expectedNewest, updateFragment)
			}

		})
	}

}

// TestHelperProcess isn't a real test. It's used as a helper process
// to make a portableish binary. See
// https://github.com/golang/go/blob/master/src/os/exec/exec_test.go#L724
// and https://npf.io/2015/06/testing-exec-command/
func TestHelperProcess(t *testing.T) {
	t.Parallel()

	// find out magic arguments
	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}
	if len(args) == 0 {
		// Indicates an error, or just being run in the test suite.
		return
	}

	switch args[0] {
	case "sleep":
		select {}
	case "exit0":
		os.Exit(0)
	case "exit1":
		os.Exit(1)
	case "exit2":
		os.Exit(2)
	}

	// default behavior nothing
}

func windowsAddExe(in string) string {
	if runtime.GOOS == "windows" {
		return in + ".exe"
	}

	return in
}
