package allowedcmd

import (
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/kolide/kit/ulid"
	"github.com/stretchr/testify/require"
)

func TestEcho(t *testing.T) {
	t.Parallel()

	// echo is the only one available on all platforms and likely to be available in CI
	tracedCmd, err := Echo.Cmd(t.Context(), "hello")
	require.NoError(t, err)
	require.Contains(t, tracedCmd.Path, "echo")
	require.Contains(t, tracedCmd.Args, "hello")
}

func TestWithEnv_posix(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip()
	}

	random := ulid.New()

	testcmd := newAllowedCommand("/usr/bin/printenv").WithEnv("CI_TEST_COMMANDS=" + random)
	tracedCmd, err := testcmd.Cmd(t.Context(), "CI_TEST_COMMANDS")
	require.NoError(t, err)

	require.Contains(t, tracedCmd.Env, "CI_TEST_COMMANDS="+random)

	output, err := tracedCmd.CombinedOutput()
	require.NoError(t, err)
	require.Contains(t, string(output), random)
}

func TestWithEnv_windows(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip()
	}

	random := ulid.New()

	testcmd := newAllowedCommand(filepath.Join(os.Getenv("WINDIR"), "System32", "cmd.exe")).WithEnv("CI_TEST_COMMANDS=" + random)
	tracedCmd, err := testcmd.Cmd(t.Context(), "/C", "set")
	require.NoError(t, err)

	require.Contains(t, tracedCmd.Env, "CI_TEST_COMMANDS="+random)

	output, err := tracedCmd.CombinedOutput()
	require.NoError(t, err)
	require.Contains(t, string(output), random)
}

func TestIsNixOS(t *testing.T) { // nolint:paralleltest
	// Make sure we can call the function
	isNixOSOriginalValue := IsNixOS()
	require.True(t, checkedIsNixOS.Load())

	// Make sure that the value does not change
	for range 5 {
		require.Equal(t, isNixOSOriginalValue, IsNixOS())
		require.True(t, checkedIsNixOS.Load())
	}

	// Reset bools and check again
	checkedIsNixOS = &atomic.Bool{}
	isNixOS = &atomic.Bool{}
	require.Equal(t, isNixOSOriginalValue, IsNixOS())
	require.True(t, checkedIsNixOS.Load())
}

func Test_newCmd(t *testing.T) {
	t.Parallel()

	cmdPath := filepath.Join("some", "path", "to", "a", "command")
	tracedCmd := newCmd(t.Context(), nil, cmdPath)
	require.Equal(t, cmdPath, tracedCmd.Path)
}

func Test_findExecutable_doesNotSearchPathOnNonNixOS(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "linux" {
		t.SkipNow()
	}

	// Use a command that has a single path that doesn't exist
	nonexistent, err := findExecutable([]string{"/not/the/real/path/to/bash"})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrCommandNotFound)
	require.Empty(t, nonexistent)
}
