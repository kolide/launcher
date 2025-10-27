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

	// Exec expects the process to continue running (because it expects to be running launcher),
	// so any exit that is not a subcommand or the windows "svc" subcommand, will be an error. Therefore, we expect an error here.
	err := Exec(t.Context(), slogger, command, args, os.Environ(), false)
	require.Error(t, err)

	// We should expect at least SOMETHING to be logged on Windows
	if runtime.GOOS == "windows" {
		require.Greater(t, len(logBytes.String()), 0)
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

	// Exec expects the process to continue running (because it expects to be running launcher),
	// so any exit that is not a subcommand or the windows "svc" subcommand, will be an error. Therefore, we expect an error here.
	err := Exec(t.Context(), slogger, command, args, os.Environ(), true)
	require.NoError(t, err)

	// We should expect at least SOMETHING to be logged
	require.Greater(t, len(logBytes.String()), 0)
}
