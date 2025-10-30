//go:build windows
// +build windows

package execwrapper

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/kolide/launcher/ee/gowrapper"
)

func Exec(ctx context.Context, slogger *slog.Logger, argv0 string, argv []string, envv []string) error {
	cmd := exec.CommandContext(ctx, argv0, argv[1:]...) //nolint:forbidigo // execwrapper is used exclusively to exec launcher, and we trust the autoupdate library to find the correct path.
	cmd.Env = envv

	// Set up our slogger with context about the command
	fullCmd := strings.Join(cmd.Args, " ")
	slogger = slogger.With("cmd", fullCmd, "component", "execwrapper")
	slogger.Log(ctx, slog.LevelInfo,
		"preparing to run command",
	)

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout

	// Set up log processing for stderr, so we don't lose it
	stdErr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("getting stderr pipe: %w", err)
	}
	gowrapper.Go(ctx, slogger, func() {
		logStderr(ctx, slogger, stdErr)
	})

	// Now run it. This is faking exec, we need to distinguish
	// between a failure to execute, and a failure in the called program.
	// I think https://github.com/golang/go/issues/26539 adds this functionality.
	err = cmd.Run()
	slogger.Log(ctx, slog.LevelError,
		"command terminated",
		"exit_code", cmd.ProcessState.ExitCode(),
		"process_state", cmd.ProcessState.String(),
		"err", err,
	)

	if cmd.ProcessState.ExitCode() == -1 {
		return fmt.Errorf("execing %s returned exit code -1 and state %s: %w", fullCmd, cmd.ProcessState.String(), err)
	}

	return fmt.Errorf("exec completed with exit code %d", cmd.ProcessState.ExitCode())
}

// logStderr reads from the given stdErr pipe and logs all lines from it until the pipe closes.
// This is particularly useful for capturing any launcher panics.
func logStderr(ctx context.Context, slogger *slog.Logger, stdErr io.ReadCloser) {
	slogger = slogger.With("subcomponent", "cmd_stderr")
	scanner := bufio.NewScanner(stdErr)

	for scanner.Scan() {
		logLine := scanner.Text()
		slogger.Log(ctx, slog.LevelError, logLine) // nolint:sloglint // it's fine to not have a constant or literal here
	}

	slogger.Log(ctx, slog.LevelDebug,
		"ending stderr logging",
	)
}
