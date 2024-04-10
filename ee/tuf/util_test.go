package tuf

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_executableLocation(t *testing.T) {
	t.Parallel()

	updateDir := t.TempDir()

	var expectedOsquerydLocation string
	var expectedLauncherLocation string
	switch runtime.GOOS {
	case "darwin":
		expectedOsquerydLocation = filepath.Join(updateDir, "osquery.app", "Contents", "MacOS", "osqueryd")
		expectedLauncherLocation = filepath.Join(updateDir, "Kolide.app", "Contents", "MacOS", "launcher")
	case "windows":
		expectedOsquerydLocation = filepath.Join(updateDir, "osqueryd.exe")
		expectedLauncherLocation = filepath.Join(updateDir, "launcher.exe")
	case "linux":
		expectedOsquerydLocation = filepath.Join(updateDir, "osqueryd")
		expectedLauncherLocation = filepath.Join(updateDir, "launcher")
	}

	require.NoError(t, os.MkdirAll(filepath.Dir(expectedOsquerydLocation), 0755))
	f, err := os.Create(expectedOsquerydLocation)
	require.NoError(t, err)
	f.Close()

	osquerydLocation := executableLocation(updateDir, "osqueryd")
	require.Equal(t, expectedOsquerydLocation, osquerydLocation)

	launcherLocation := executableLocation(updateDir, "launcher")
	require.Equal(t, expectedLauncherLocation, launcherLocation)
}

func Test_executableLocation_nonAppBundle(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "darwin" {
		t.SkipNow()
	}

	updateDir := t.TempDir()
	expectedOsquerydLocation := filepath.Join(updateDir, "osqueryd")

	f, err := os.Create(expectedOsquerydLocation)
	require.NoError(t, err)
	f.Close()

	osquerydLocation := executableLocation(updateDir, "osqueryd")
	require.Equal(t, expectedOsquerydLocation, osquerydLocation)
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
			err := CheckExecutable(context.TODO(), targetExe, "-test.run=TestHelperProcess", "--", tt.testName)
			if tt.expectedErr {
				require.Error(t, err, tt.testName)

				// As a bonus, we can test that if we call os.Args[0],
				// we still don't get an error. This is because we
				// trigger the match against os.Executable and don't
				// invoked. This is here, and not a dedicated test,
				// because we ensure the same test arguments.
				require.NoError(t, CheckExecutable(context.TODO(), os.Args[0], "-test.run=TestHelperProcess", "--", tt.testName), "calling self with %s", tt.testName)
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
		CheckExecutable(context.TODO(), truncatedBinary.Name(), "-test.run=TestHelperProcess", "--", "exit0"),
		"truncated binary")
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

type setupState int

const (
	emptySetup setupState = iota
	emptyUpdateDirs
	nonExecutableUpdates
	executableUpdates
	truncatedUpdates
)

// This suffix is added to the binary path to find the updates
const updateDirSuffix = "-updates"

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
		time.Sleep(10 * time.Second)
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

func Test_checkExecutablePermissions(t *testing.T) {
	t.Parallel()

	require.Error(t, checkExecutablePermissions(""), "passing empty string")
	require.Error(t, checkExecutablePermissions("/random/path/should/not/exist"), "passing non-existent file path")

	// Setup the tests
	tmpDir := t.TempDir()

	require.Error(t, checkExecutablePermissions(tmpDir), "directory should not be executable")

	dotExe := ""
	if runtime.GOOS == "windows" {
		dotExe = ".exe"
	}

	fileName := filepath.Join(tmpDir, "file") + dotExe
	tmpFile, err := os.Create(fileName)
	require.NoError(t, err, "os create")
	tmpFile.Close()

	hardLink := filepath.Join(tmpDir, "hardlink") + dotExe
	require.NoError(t, os.Link(fileName, hardLink), "making link")

	symLink := filepath.Join(tmpDir, "symlink") + dotExe
	require.NoError(t, os.Symlink(fileName, symLink), "making symlink")

	// windows doesn't have an executable bit
	if runtime.GOOS == "windows" {
		require.NoError(t, checkExecutablePermissions(fileName), "plain file")
		require.NoError(t, checkExecutablePermissions(hardLink), "hard link")
		require.NoError(t, checkExecutablePermissions(symLink), "symlink")
	} else {
		require.Error(t, checkExecutablePermissions(fileName), "plain file")
		require.Error(t, checkExecutablePermissions(hardLink), "hard link")
		require.Error(t, checkExecutablePermissions(symLink), "symlink")

		require.NoError(t, os.Chmod(fileName, 0755))
		require.NoError(t, checkExecutablePermissions(fileName), "plain file")
		require.NoError(t, checkExecutablePermissions(hardLink), "hard link")
		require.NoError(t, checkExecutablePermissions(symLink), "symlink")
	}
}
