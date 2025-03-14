package tuf

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kolide/launcher/pkg/traces"
)

// CheckExecutable tests whether something is an executable. It
// examines permissions, mode, and tries to exec it directly.
func CheckExecutable(ctx context.Context, slogger *slog.Logger, potentialBinary string, args ...string) error {
	ctx, span := traces.StartSpan(ctx, "binary_path", potentialBinary)
	defer span.End()

	slogger = slogger.With("subcomponent", "CheckExecutable", "binary_path", potentialBinary, "args", fmt.Sprintf("%+v", args))

	if err := checkExecutablePermissions(ctx, potentialBinary); err != nil {
		slogger.Log(ctx, slog.LevelWarn,
			"failed executable permissions check",
			"err", err,
		)
		return fmt.Errorf("checking executable permissions: %w", err)
	}

	// If we can determine that the requested executable is
	// ourself, don't try to exec. It's needless, and a potential
	// fork bomb. Ignore errors, either we get an answer or we don't.
	selfPath, _ := os.Executable()
	if filepath.Clean(selfPath) == filepath.Clean(potentialBinary) {
		slogger.Log(ctx, slog.LevelInfo,
			"binary path matches current executable path, no need to exec",
			"self_path", selfPath,
		)
		return nil
	}

	// If we get ETXTBSY error when execing, this could be because this
	// binary is freshly downloaded. Retry a small number of times only
	// in that circumstance.
	// See: https://github.com/golang/go/issues/22315
	for i := 0; i < 3; i += 1 {
		out, execErr := runExecutableCheck(ctx, potentialBinary, args...)
		if execErr != nil && errors.Is(execErr, syscall.ETXTBSY) {
			continue
		}

		if err := supressRoutineErrors(ctx, execErr, out); err != nil {
			slogger.Log(ctx, slog.LevelWarn,
				"executable check returned error",
				"err", err,
				"exec_err", execErr,
				"command_output", string(out),
			)
			return err
		}
		slogger.Log(ctx, slog.LevelInfo,
			"successfully checked executable",
		)
		return nil
	}

	slogger.Log(ctx, slog.LevelWarn,
		"received ETXTBSY multiple times when running executable check",
	)

	return fmt.Errorf("could not exec %s despite retries due to text file busy", potentialBinary)
}

// runExecutableCheck runs a single exec against the given binary and returns the result.
func runExecutableCheck(ctx context.Context, potentialBinary string, args ...string) ([]byte, error) {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, potentialBinary, args...) //nolint:forbidigo // We trust the autoupdate library to find the correct location so we don't need allowedcmd

	// Set env, this should prevent launcher for fork-bombing
	cmd.Env = append(cmd.Env, "LAUNCHER_SKIP_UPDATES=TRUE")

	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("timeout when checking executable: %w", ctx.Err())
	}

	return out, err
}

// supressRoutineErrors attempts to tell whether the error was a
// program that has executed, and then exited, vs one that's execution
// was entirely unsuccessful. This differentiation allows us to
// detect, and recover, from corrupt updates vs something in-app.
func supressRoutineErrors(ctx context.Context, err error, combinedOutput []byte) error {
	_, span := traces.StartSpan(ctx)
	defer span.End()

	if err == nil {
		return nil
	}

	// Suppress exit codes of 1 or 2. These are generally indicative of
	// an unknown command line flag, _not_ a corrupt download. (exit
	// code 0 will be nil, and never trigger this block)
	if exitError, ok := err.(*exec.ExitError); ok {
		if exitError.ExitCode() == 1 || exitError.ExitCode() == 2 {
			// suppress these
			return nil
		}
	}
	return fmt.Errorf("exec error: output: `%s`, err: %w", string(combinedOutput), err)
}
