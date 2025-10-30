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

	// by not setting command and args on non-windows, we will trigger an error in syscall.Exec
	// otherwise we'll never return from Exec on non-windows because syscall.Exec replaces the current process
	command := ""
	args := []string{}

	// on windows we want to actually run something that will exit cleanly
	if runtime.GOOS == "windows" {
		command = "cmd.exe"
		// Exec removes the first argument, so add a blank to make sure our args get processed
		args = []string{"", "/c", "echo", "test string"}
	}

	// non-windows will give us an error here because syscall.Exec will fail
	// echo on windows will exit cleanly, but we expect an error because we set isNonSvcSubCommand to false
	err := Exec(t.Context(), slogger, command, args, os.Environ(), false)
	require.Error(t, err)

	if runtime.GOOS == "windows" {
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

	command := "/bin/echo"
	args := []string{"echo", "test string"}

	if runtime.GOOS == "windows" {
		command = "cmd.exe"
		// Exec removes the first argument, so add a blank to make sure our args get processed
		args = []string{"", "/c", "echo", "test string"}
	}

	// flag this call as a non-svc subcommand so if it exist with out error, we do not return an error
	err := Exec(t.Context(), slogger, command, args, os.Environ(), true)

	// on non-windows, we never actually get to this line, because syscall.Exec replaces the current process
	require.NoError(t, err)

	// We should expect at least SOMETHING to be logged on Windows
	require.Contains(t, logBytes.String(), "exit status 0")
}
