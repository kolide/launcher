package tuf

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/kolide/launcher/pkg/log/multislogger"
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
	tmpDir := t.TempDir()
	binaryName := windowsAddExe("testbinary")
	targetExe := filepath.Join(tmpDir, binaryName)
	require.NoError(t, os.MkdirAll(filepath.Dir(tmpDir), 0755))

	// We use the golang test binary here so we can run `TestHelperProcess` with the desired outcome
	require.NoError(t, os.Symlink(os.Args[0], targetExe))
	require.NoError(t, os.Chmod(targetExe, 0755))

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
			expectedErr: true,
		},
		{
			testName:    "exit2",
			expectedErr: true,
		},
		{
			testName:    "sleep",
			expectedErr: true,
		},
	}

	for _, tt := range tests { // nolint:paralleltest
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			err := CheckExecutable(context.TODO(), multislogger.NewNopLogger(), targetExe, "-test.run=TestHelperProcess", "--", tt.testName)
			if tt.expectedErr {
				require.Error(t, err, tt.testName)

				// As a bonus, we can test that if we call os.Args[0],
				// we still don't get an error. This is because we
				// trigger the match against os.Executable and don't
				// invoked. This is here, and not a dedicated test,
				// because we ensure the same test arguments.
				require.NoError(t, CheckExecutable(context.TODO(), multislogger.NewNopLogger(), os.Args[0], "-test.run=TestHelperProcess", "--", tt.testName), "calling self with %s", tt.testName)
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

	copyFileTruncated(t, truncatedBinary.Name(), os.Args[0])
	require.NoError(t, os.Chmod(truncatedBinary.Name(), 0755))

	require.Error(t,
		CheckExecutable(context.TODO(), multislogger.NewNopLogger(), truncatedBinary.Name(), "-test.run=TestHelperProcess", "--", "exit0"),
		"truncated binary")
}

// copyFile copies half of the file from srcPath to dstPath.
func copyFileTruncated(t *testing.T, dstPath, srcPath string) {
	src, err := os.Open(srcPath)
	require.NoError(t, err, "opening src")
	defer src.Close()

	dst, err := os.Create(dstPath)
	require.NoError(t, err, "opening dest")
	defer dst.Close()

	stat, err := src.Stat()
	require.NoError(t, err, "statting src")

	_, err = io.CopyN(dst, src, stat.Size()/2)
	require.NoError(t, err, "copying src to dest")
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
		os.Exit(0) //nolint:forbidigo // Fine to use os.Exit inside tests
	case "exit1":
		os.Exit(1) //nolint:forbidigo // Fine to use os.Exit inside tests
	case "exit2":
		os.Exit(2) //nolint:forbidigo // Fine to use os.Exit inside tests
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

	require.Error(t, checkExecutablePermissions(context.TODO(), ""), "passing empty string")
	require.Error(t, checkExecutablePermissions(context.TODO(), "/random/path/should/not/exist"), "passing non-existent file path")

	// Setup the tests
	tmpDir := t.TempDir()

	require.Error(t, checkExecutablePermissions(context.TODO(), tmpDir), "directory should not be executable")

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
		require.NoError(t, checkExecutablePermissions(context.TODO(), fileName), "plain file")
		require.NoError(t, checkExecutablePermissions(context.TODO(), hardLink), "hard link")
		require.NoError(t, checkExecutablePermissions(context.TODO(), symLink), "symlink")
	} else {
		require.Error(t, checkExecutablePermissions(context.TODO(), fileName), "plain file")
		require.Error(t, checkExecutablePermissions(context.TODO(), hardLink), "hard link")
		require.Error(t, checkExecutablePermissions(context.TODO(), symLink), "symlink")

		require.NoError(t, os.Chmod(fileName, 0755))
		require.NoError(t, checkExecutablePermissions(context.TODO(), fileName), "plain file")
		require.NoError(t, checkExecutablePermissions(context.TODO(), hardLink), "hard link")
		require.NoError(t, checkExecutablePermissions(context.TODO(), symLink), "symlink")
	}
}
