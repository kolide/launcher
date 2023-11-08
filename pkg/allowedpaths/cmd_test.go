package allowedpaths

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_newCmd(t *testing.T) {
	t.Parallel()

	cmdPath := filepath.Join("some", "path", "to", "a", "command")
	cmd := newCmd(cmdPath)
	require.Equal(t, cmdPath, cmd.Path)
}

func Test_validatedPath(t *testing.T) {
	t.Parallel()

	var cmdPath string
	switch runtime.GOOS {
	case "darwin", "linux":
		cmdPath = "/bin/bash"
	case "windows":
		cmdPath = `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`
	}

	p, err := validatedPath(cmdPath)

	require.NoError(t, err)
	require.Equal(t, cmdPath, p)
}
