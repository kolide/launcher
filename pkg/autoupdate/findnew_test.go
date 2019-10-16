package autoupdate

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
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
	binaryPath, err := os.Executable()
	require.NoError(t, err)
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

	expectedNewest := filepath.Join(updatesDir, "3", filepath.Base(binaryPath))

	f, err := os.Create(expectedNewest)
	require.NoError(t, err)
	f.Close()
	require.NoError(t, os.Chmod(f.Name(), 0755))

	{
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
		{in: "/a/path/launcher", out: "/a/path"},
		{in: "/a/path/launcher-updates/1569339163/launcher", out: "/a/path"},
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

	require.Equal(t, expectedNewest, FindNewest(ctx, binaryPath), "Should find number 5")
	require.Equal(t, expectedNewest, FindNewest(ctx, expectedNewest), "already running the newest")

}

func TestFindNewestCleanup(t *testing.T) {
	t.Parallel()

	tmpDir, binaryName, cleanupFunc := setupTestDir(t, executableUpdates)
	_ = cleanupFunc
	//defer cleanupFunc()
	ctx := context.TODO()
	binaryPath := filepath.Join(tmpDir, binaryName)
	updatesDir := fmt.Sprintf("%s%s", binaryPath, updateDirSuffix)

	expectedNewest := filepath.Join(updatesDir, "5", "binary")
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
		require.Equal(t, 2, len(updatesOnDisk), "after delete")
		require.Equal(t, expectedNewest, FindNewest(ctx, binaryPath), "Should find number 5")
	}

}

type setupState int

const (
	emptySetup setupState = iota
	emptyUpdateDirs
	nonExecutableUpdates
	executableUpdates
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

	// Create the fake binary
	binaryName := "binary"
	if runtime.GOOS == "windows" {
		binaryName = binaryName + ".exe"
	}
	binaryPath := filepath.Join(tmpDir, binaryName)
	updatesDir := fmt.Sprintf("%s%s", binaryPath, updateDirSuffix)

	{
		tmpFile, err := os.Create(binaryPath)
		require.NoError(t, err, "os create")
		tmpFile.Close()
		require.NoError(t, os.Chmod(binaryPath, 0755))
	}

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
		f, err := os.Create(filepath.Join(updatesDir, n, binaryName))
		require.NoError(t, err)
		f.Close()
	}

	if stage <= nonExecutableUpdates {
		return tmpDir, binaryName, cleanupFunc
	}

	for _, n := range []string{"2", "5", "3", "1"} {
		require.NoError(t, os.Chmod(filepath.Join(updatesDir, n, binaryName), 0755))
	}

	return tmpDir, binaryName, cleanupFunc

}
