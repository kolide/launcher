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

	// Exec expects the process to continue running (because it expects to be running launcher),
	// so any exit that is not a subcommand or the windows "svc" subcommand, will be an error. Therefore, we expect an error here.
	err := Exec(t.Context(), slogger, "/bin/echo", []string{"test string"}, os.Environ(), false)
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

	// Exec expects the process to continue running (because it expects to be running launcher),
	// so any exit that is not a subcommand or the windows "svc" subcommand, will be an error. Therefore, we expect an error here.
	err := Exec(t.Context(), slogger, "/bin/echo", []string{"test string"}, os.Environ(), true)
	require.NoError(t, err)

	// We should expect at least SOMETHING to be logged on Windows
	if runtime.GOOS == "windows" {
		require.Greater(t, len(logBytes.String()), 0)
	}
}
