package allowedcmd

import (
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEcho(t *testing.T) {
	t.Parallel()

	// echo is the only one available on all platforms and likely to be available in CI
	tracedCmd, err := Echo(t.Context(), "hello")
	require.NoError(t, err)
	require.Contains(t, tracedCmd.Path, "echo")
	require.Contains(t, tracedCmd.Args, "hello")
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
	tracedCmd := newCmd(t.Context(), cmdPath)
	require.Equal(t, cmdPath, tracedCmd.Path)
}

func Test_validatedCommand(t *testing.T) {
	t.Parallel()

	var cmdPath string
	switch runtime.GOOS {
	case "darwin", "linux":
		cmdPath = "/bin/bash"
	case "windows":
		cmdPath = `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`
	}

	tracedCmd, err := validatedCommand(t.Context(), cmdPath)

	require.NoError(t, err)
	require.Equal(t, cmdPath, tracedCmd.Path)
}

func Test_validatedCommand_doesNotSearchPathOnNonNixOS(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "linux" {
		t.SkipNow()
	}

	cmdPath := "/not/the/real/path/to/bash"
	_, err := validatedCommand(t.Context(), cmdPath)

	require.Error(t, err)
}
