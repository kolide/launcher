package execwrapper

import (
	"log/slog"
	"os"
	"runtime"
	"testing"

	"github.com/kolide/launcher/pkg/threadsafebuffer"
	"github.com/stretchr/testify/require"
)

func TestExec(t *testing.T) {
	t.Parallel()

	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	var command string
	var args []string

	if runtime.GOOS == "windows" {
		command = "cmd.exe"
		args = []string{"/c", "echo", "test string"}
	} else {
		command = "/bin/sh"
		args = []string{"-c", "echo test string"}
	}

	args = append([]string{""}, args...)

	// Exec expects the process to continue running (because it expects to be running launcher),
	// so any exit that is not a subcommand or the windows "svc" subcommand, will be an error. Therefore, we expect an error here.
	err := Exec(t.Context(), slogger, command, args, os.Environ(), false)
	require.Error(t, err)

	// We should expect at least SOMETHING to be logged on Windows
	if runtime.GOOS == "windows" {
		require.Greater(t, len(logBytes.String()), 0)

		// your eyes do not decive you, we expect an exit status 0 in the logs even though Exec returned an error
		// this is because Exec is used by the main launcher process to run new versions of launcher, if the newer version
		// exits for ANY reason we want that treated as an error so the main process will exit with an error so that
		// windows service manager knows to restart it
		require.Contains(t, logBytes.String(), "exit status 0")
	}
}

func TestExecSubcommand(t *testing.T) {
	t.Parallel()

	var logBytes threadsafebuffer.ThreadSafeBuffer
	slogger := slog.New(slog.NewTextHandler(&logBytes, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	var command string
	var args []string

	if runtime.GOOS == "windows" {
		command = "cmd.exe"
		args = []string{"/c", "echo", "test string"}
	} else {
		command = "/bin/sh"
		args = []string{"-c", "echo test string"}
	}

	// Exec removes the first argument, so add a blank to make sure our args get processed
	args = append([]string{""}, args...)

	// flag this call as a non-svc subcommand so if it exist with out error, we do not return an error
	err := Exec(t.Context(), slogger, command, args, os.Environ(), true)
	require.NoError(t, err)

	// We should expect at least SOMETHING to be logged on Windows
	if runtime.GOOS == "windows" {
		require.Greater(t, len(logBytes.String()), 0)
		require.Contains(t, logBytes.String(), "exit status 0")
	}
}
