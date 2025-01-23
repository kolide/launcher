package allowedcmd

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEcho(t *testing.T) {
	t.Parallel()

	// echo is the only one available on all platforms and likely to be available in CI
	tracedCmd, err := Echo(context.TODO(), "hello")
	require.NoError(t, err)
	require.Contains(t, tracedCmd.Path, "echo")
	require.Contains(t, tracedCmd.Args, "hello")
}

func Test_newCmd(t *testing.T) {
	t.Parallel()

	cmdPath := filepath.Join("some", "path", "to", "a", "command")
	tracedCmd := newCmd(context.TODO(), cmdPath)
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

	tracedCmd, err := validatedCommand(context.TODO(), cmdPath)

	require.NoError(t, err)
	require.Equal(t, cmdPath, tracedCmd.Path)
}

func Test_validatedCommand_doesNotSearchPathOnNonNixOS(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "linux" {
		t.SkipNow()
	}

	cmdPath := "/not/the/real/path/to/bash"
	_, err := validatedCommand(context.TODO(), cmdPath)

	require.Error(t, err)
}
