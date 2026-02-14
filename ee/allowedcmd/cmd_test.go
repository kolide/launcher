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

func TestWithEnv(t *testing.T) {
	t.Parallel()

	randomString := ulid.New()

	// Windows has a different mechanism to print envs than posix does, so we vary here
	cmdpath := "/usr/bin/printenv"
	cmdargs := []string{"CI_TEST_COMMANDS"}
	if runtime.GOOS == "windows" {
		// this is the same as CommandPrompt, but that's not defined on posix machines, so it's simpler just to duplicate here
		cmdpath = filepath.Join(os.Getenv("WINDIR"), "System32", "cmd.exe")
		cmdargs = []string{"/C", "set"}
	}

	testcmd := newAllowedCommand(cmdpath).WithEnv("CI_TEST_COMMANDS=" + randomString)

	tracedCmd, err := testcmd.Cmd(t.Context(), cmdargs...)
	require.NoError(t, err)
	require.Contains(t, tracedCmd.Env, "CI_TEST_COMMANDS="+randomString)

	output, err := tracedCmd.CombinedOutput()
	require.NoError(t, err)
	require.Contains(t, string(output), randomString)
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
