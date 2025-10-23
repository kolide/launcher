//go:build windows
// +build windows

package execwrapper

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

func Exec(ctx context.Context, slogger *slog.Logger, argv0 string, argv []string, envv []string) error {
	cmd := exec.CommandContext(ctx, argv0, argv[1:]...) //nolint:forbidigo // execwrapper is used exclusively to exec launcher, and we trust the autoupdate library to find the correct path.
	cmd.Env = envv

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fullCmd := strings.Join(cmd.Args, " ")
	slogger = slogger.With("cmd", fullCmd, "component", "execwrapper")
	slogger.Log(ctx, slog.LevelInfo,
		"preparing to run command",
	)

	// Now run it. This is faking exec, we need to distinguish
	// between a failure to execute, and a failure in the called program.
	// I think https://github.com/golang/go/issues/26539 adds this functionality.
	err := cmd.Run()
	slogger.Log(ctx, slog.LevelError,
		"command terminated",
		"exit_code", cmd.ProcessState.ExitCode(),
		"err", err,
	)

	if cmd.ProcessState.ExitCode() == -1 {
		return fmt.Errorf("execing %s returned exit code -1 and state %s: %w", fullCmd, cmd.ProcessState.String(), err)
	}

	return fmt.Errorf("exec completed with exit code %d", cmd.ProcessState.ExitCode())
}
