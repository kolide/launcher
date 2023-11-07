package allowedpaths

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCommandWithLookup(t *testing.T) {
	t.Parallel()

	var cmdName string
	if runtime.GOOS == "windows" {
		cmdName = "powershell.exe"
	} else {
		// On Linux and Darwin we expect ps to be available, even on the CI runner
		cmdName = "ps"
	}

	cmd, err := CommandWithLookup(cmdName)
	require.NoError(t, err, "expected to find command")
	require.True(t, strings.HasSuffix(cmd.Path, cmdName))
}

func TestCommandWithLookup_NotFound(t *testing.T) {
	t.Parallel()

	cmdName := "some-cmd"
	_, err := CommandWithLookup(cmdName)
	require.Error(t, err, "should not have found cmd")
}

func TestCommandWithPath_Allowed(t *testing.T) {
	t.Parallel()

	for cmdName, cmdPaths := range knownPaths {
		cmdName := cmdName
		cmdPaths := cmdPaths
		t.Run(cmdName, func(t *testing.T) {
			t.Parallel()

			for p := range cmdPaths {
				cmd, err := CommandWithPath(p)
				require.NoError(t, err, "expected no error with known path")
				require.Equal(t, p, cmd.Path)
			}
		})
	}
}

func TestCommandWithPath_Denied(t *testing.T) {
	t.Parallel()

	// Unknown command
	_, err := CommandWithPath(filepath.Join("some", "path", "to", "not-a-real-command"))
	require.Error(t, err, "expected unknown command to be denylisted")

	// Known command, unknown path
	for cmdName := range knownPaths {
		_, err := CommandWithPath(filepath.Join("some", "incorrect", "path", "to", cmdName))
		require.Error(t, err, "expected unknown path to be denylisted")

		// We don't need to perform this same test against every known path
		break
	}
}

func TestCommandWithPath_AutoupdatePaths(t *testing.T) {
	t.Parallel()

	for _, b := range []string{"launcher", "launcher.exe", "osqueryd", "osqueryd.exe"} {
		b := b
		t.Run(b, func(t *testing.T) {
			t.Parallel()

			autoupdatedBinaryPath := filepath.Join("some", "autoupdate", "path", "to", b)
			cmd, err := CommandWithPath(autoupdatedBinaryPath)
			require.NoError(t, err, "expected autoupdated binaries to be allowed")
			require.Equal(t, autoupdatedBinaryPath, cmd.Path)
		})
	}
}

func TestCommandWithPath_KnownPrefix(t *testing.T) {
	t.Parallel()

	for cmdName := range knownPaths {
		cmdName := cmdName
		t.Run(cmdName, func(t *testing.T) {
			t.Parallel()

			for _, p := range knownPathPrefixes {
				pathToCmdWithKnownPrefix := filepath.Join(p, cmdName)
				cmd, err := CommandWithPath(pathToCmdWithKnownPrefix)
				require.NoError(t, err, "expected no error with known command and directory prefix")
				require.Equal(t, pathToCmdWithKnownPrefix, cmd.Path)
			}
		})
	}
}

func TestCommandContextWithLookup(t *testing.T) {
	t.Parallel()

	var cmdName string
	if runtime.GOOS == "windows" {
		cmdName = "powershell.exe"
	} else {
		// On Linux and Darwin we expect ps to be available, even on the CI runner
		cmdName = "ps"
	}

	cmd, err := CommandContextWithLookup(context.TODO(), cmdName)
	require.NoError(t, err, "expected to find command")
	require.True(t, strings.HasSuffix(cmd.Path, cmdName))
	require.NotNil(t, cmd.Cancel, "context not set on command")
}

func TestCommandContextWithLookup_NotFound(t *testing.T) {
	t.Parallel()

	cmdName := "some-cmd"
	_, err := CommandContextWithLookup(context.TODO(), cmdName)
	require.Error(t, err, "should not have found cmd")
}

func TestCommandContextWithPath_Allowed(t *testing.T) {
	t.Parallel()

	for cmdName, cmdPaths := range knownPaths {
		cmdName := cmdName
		cmdPaths := cmdPaths
		t.Run(cmdName, func(t *testing.T) {
			t.Parallel()

			for p := range cmdPaths {
				cmd, err := CommandContextWithPath(context.TODO(), p)
				require.NoError(t, err, "expected no error with known path")
				require.Equal(t, p, cmd.Path)
				require.NotNil(t, cmd.Cancel, "context not set on command")
			}
		})
	}
}

func TestCommandContextWithPath_Denied(t *testing.T) {
	t.Parallel()

	// Unknown command
	_, err := CommandContextWithPath(context.TODO(), filepath.Join("some", "path", "to", "not-a-real-command"))
	require.Error(t, err, "expected unknown command to be denylisted")

	// Known command, unknown path
	for cmdName := range knownPaths {
		_, err := CommandContextWithPath(context.TODO(), filepath.Join("some", "incorrect", "path", "to", cmdName))
		require.Error(t, err, "expected unknown path to be denylisted")

		// We don't need to perform this same test against every known path
		break
	}
}

func TestCommandContextWithPath_AutoupdatePaths(t *testing.T) {
	t.Parallel()

	for _, b := range []string{"launcher", "launcher.exe", "osqueryd", "osqueryd.exe"} {
		b := b
		t.Run(b, func(t *testing.T) {
			t.Parallel()

			autoupdatedBinaryPath := filepath.Join("some", "autoupdate", "path", "to", b)
			cmd, err := CommandContextWithPath(context.TODO(), autoupdatedBinaryPath)
			require.NoError(t, err, "expected autoupdated binaries to be allowed")
			require.Equal(t, autoupdatedBinaryPath, cmd.Path)
			require.NotNil(t, cmd.Cancel, "context not set on command")
		})
	}
}

func TestCommandContextWithPath_KnownPrefix(t *testing.T) {
	t.Parallel()

	for cmdName := range knownPaths {
		cmdName := cmdName
		t.Run(cmdName, func(t *testing.T) {
			t.Parallel()

			for _, p := range knownPathPrefixes {
				pathToCmdWithKnownPrefix := filepath.Join(p, cmdName)
				cmd, err := CommandContextWithPath(context.TODO(), pathToCmdWithKnownPrefix)
				require.NoError(t, err, "expected no error with known command and directory prefix")
				require.Equal(t, pathToCmdWithKnownPrefix, cmd.Path)
				require.NotNil(t, cmd.Cancel, "context not set on command")
			}
		})
	}
}
