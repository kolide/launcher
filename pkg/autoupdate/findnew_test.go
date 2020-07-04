package autoupdate

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/pkg/errors"
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
		{in: "/a/path", out: "/a/path-updates"},
		{in: "/a/path-updates", out: "/a/path-updates"},
		{in: "/a/path-updates/1234/binary", out: "/a/path-updates"},
		{in: "/a/path/foo/bar-updates/1234/binary", out: "/a/path/foo/bar-updates"},
		{in: "/a/b-updates/123/b-updates/456/b", out: "/a/b-updates"},
		{in: "/a/path/", out: "/a/path-updates"},
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
		{in: "/a/path/launcher", out: filepath.Clean("/a/path")},
		{in: "/a/path/launcher-updates/1569339163/launcher", out: filepath.Clean("/a/path")},
	}

	for _, tt := range tests {
		require.Equal(t, tt.out, FindBaseDir(tt.in), "input: %s", tt.in)
	}

}

func TestFindNewestEmpty(t *testing.T) {
	t.Parallel()

	tmpDir, binaryName, cleanupFunc := setupTestDir(t, emptySetup)
	defer cleanupFunc()
	ctx := context.TODO()
	binaryPath := filepath.Join(tmpDir, binaryName)

	// Basic tests, test with binary and no updates
	require.Empty(t, FindNewest(ctx, ""), "passing empty string")
	require.Empty(t, FindNewest(ctx, tmpDir), "passing directory as arg")
	require.Equal(t, binaryPath, FindNewest(ctx, binaryPath), "no update directory")
}

func TestFindNewestEmptyUpdateDirs(t *testing.T) {
	t.Parallel()

	tmpDir, binaryName, cleanupFunc := setupTestDir(t, emptyUpdateDirs)
	defer cleanupFunc()
	ctx := context.TODO()
	binaryPath := filepath.Join(tmpDir, binaryName)

	require.Equal(t, binaryPath, FindNewest(ctx, binaryPath), "update dir, but no updates")
}

func TestFindNewestNonExecutable(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Windows doesn't use executable bit")
	}

	tmpDir, binaryName, cleanupFunc := setupTestDir(t, nonExecutableUpdates)
	defer cleanupFunc()
	ctx := context.TODO()
	binaryPath := filepath.Join(tmpDir, binaryName)
	updatesDir := fmt.Sprintf("%s%s", binaryPath, updateDirSuffix)

	require.Equal(t, binaryPath, FindNewest(ctx, binaryPath), "update dir, but only plain files")

	require.NoError(t, os.Chmod(filepath.Join(updatesDir, "1", "binary"), 0755))
	require.Equal(t,
		filepath.Join(updatesDir, "1", "binary"),
		FindNewest(ctx, binaryPath),
		"Should find number 1",
	)
}

func TestFindNewestExecutableUpdates(t *testing.T) {
	t.Parallel()

	tmpDir, binaryName, cleanupFunc := setupTestDir(t, executableUpdates)
	defer cleanupFunc()
	ctx := context.TODO()
	binaryPath := filepath.Join(tmpDir, binaryName)
	updatesDir := fmt.Sprintf("%s%s", binaryPath, updateDirSuffix)

	expectedNewest := filepath.Join(updatesDir, "5", "binary")
	if runtime.GOOS == "windows" {
		expectedNewest = expectedNewest + ".exe"
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

	tmpDir, binaryName, cleanupFunc := setupTestDir(t, executableUpdates)
	defer cleanupFunc()
	ctx := context.TODO()
	binaryPath := filepath.Join(tmpDir, binaryName)
	updatesDir := fmt.Sprintf("%s%s", binaryPath, updateDirSuffix)

	expectedNewest := filepath.Join(updatesDir, "5", "binary")
	if runtime.GOOS == "windows" {
		expectedNewest = expectedNewest + ".exe"
	}

	{
		updatesOnDisk, err := ioutil.ReadDir(updatesDir)
		require.NoError(t, err)
		require.Equal(t, 4, len(updatesOnDisk))
		require.Equal(t, expectedNewest, FindNewest(ctx, binaryPath), "Should find number 5")
	}

	{
		_ = FindNewest(ctx, binaryPath, DeleteOldUpdates())
		updatesOnDisk, err := ioutil.ReadDir(updatesDir)
		require.NoError(t, err)
		require.Equal(t, expectedNewest, FindNewest(ctx, binaryPath), "Should find number 5")
		require.Equal(t, 2, len(updatesOnDisk), "after delete")

	}
}

func TestCheckExecutableCorruptCleanup(t *testing.T) {
	t.Parallel()

	tmpDir, binaryName, cleanupFunc := setupTestDir(t, truncatedUpdates)
	defer cleanupFunc()
	ctx := context.TODO()
	binaryPath := filepath.Join(tmpDir, binaryName)
	updatesDir := fmt.Sprintf("%s%s", binaryPath, updateDirSuffix)

	expectedNewest := filepath.Join(updatesDir, "3", "binary")
	if runtime.GOOS == "windows" {
		expectedNewest = expectedNewest + ".exe"
	}

	{
		updatesOnDisk, err := ioutil.ReadDir(updatesDir)
		require.NoError(t, err)
		require.Equal(t, 4, len(updatesOnDisk))
		require.Equal(t, expectedNewest, FindNewest(ctx, binaryPath), "Should find number 3")
	}

	{
		_ = FindNewest(ctx, binaryPath, DeleteCorruptUpdates())
		updatesOnDisk, err := ioutil.ReadDir(updatesDir)
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
func setupTestDir(t *testing.T, stage setupState) (string, string, func()) {
	tmpDir, err := ioutil.TempDir("", "test-autoupdate-find-newest")
	require.NoError(t, err)

	cleanupFunc := func() {
		os.RemoveAll(tmpDir)
	}

	// Create a test binary
	binaryName := windowsAddExe("binary")
	binaryPath := filepath.Join(tmpDir, binaryName)
	updatesDir := fmt.Sprintf("%s%s", binaryPath, updateDirSuffix)

	require.NoError(t, copyFile(binaryPath, os.Args[0], false), "copy executable")
	require.NoError(t, os.Chmod(binaryPath, 0755), "chmod")

	if stage <= emptySetup {
		return tmpDir, binaryName, cleanupFunc
	}

	// make some update directories
	// (these are out of order, to jumble up the create times)
	for _, n := range []string{"2", "5", "3", "1"} {
		require.NoError(t, os.MkdirAll(filepath.Join(updatesDir, n), 0755))
	}

	if stage <= emptyUpdateDirs {
		return tmpDir, binaryName, cleanupFunc
	}

	for _, n := range []string{"2", "5", "3", "1"} {
		updatedBinaryPath := filepath.Join(updatesDir, n, binaryName)
		require.NoError(t, copyFile(updatedBinaryPath, binaryPath, false), "copy executable")
	}

	if stage <= nonExecutableUpdates {
		return tmpDir, binaryName, cleanupFunc
	}

	for _, n := range []string{"2", "5", "3", "1"} {
		require.NoError(t, os.Chmod(filepath.Join(updatesDir, n, binaryName), 0755))
	}

	if stage <= executableUpdates {
		return tmpDir, binaryName, cleanupFunc
	}

	for _, n := range []string{"5", "1"} {
		updatedBinaryPath := filepath.Join(updatesDir, n, binaryName)
		require.NoError(t, copyFile(updatedBinaryPath, binaryPath, true), "copy & truncate executable")
	}

	return tmpDir, binaryName, cleanupFunc

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
			return errors.Wrap(err, "statting srcFile")
		}

		if _, err = io.CopyN(dst, src, stat.Size()/2); err != nil {
			return errors.Wrap(err, "statting srcFile")
		}
	}

	return nil
}

func TestCheckExecutable(t *testing.T) {
	t.Parallel()

	// We need to run this from a temp dir, else the early return
	// from matching os.Executable bypasses the point of this.
	tmpDir, binaryName, cleanupFunc := setupTestDir(t, executableUpdates)
	defer cleanupFunc()
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

	for _, tt := range tests {
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
	truncatedBinary, err := ioutil.TempFile("", "test-autoupdate-check-executable-truncation")
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
		t.Run("buildTimestamp="+tt.buildTimestamp, func(t *testing.T) {
			tmpDir, binaryName, cleanupFunc := setupTestDir(t, executableUpdates)
			defer cleanupFunc()
			ctx := context.TODO()

			binaryPath := filepath.Join(tmpDir, binaryName)
			updatesDir := binaryPath + updateDirSuffix

			returnedNewest := FindNewest(
				ctx,
				binaryPath,
				overrideBuildTimestamp(tt.buildTimestamp),
				DeleteOldUpdates(),
			)

			updatesOnDisk, err := ioutil.ReadDir(updatesDir)
			require.NoError(t, err)
			require.Equal(t, tt.expectedOnDisk, len(updatesOnDisk), "remaining updates on disk")

			if tt.expectedNewest == "" {
				require.Equal(t, binaryPath, returnedNewest, "Expected to get original binary path")
			} else {
				updateFragment := strings.TrimPrefix(strings.TrimPrefix(returnedNewest, updatesDir), "/")
				expectedNewest := filepath.Join(tt.expectedNewest, "binary")
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
