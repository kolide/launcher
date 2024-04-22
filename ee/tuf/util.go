package tuf

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kolide/launcher/pkg/traces"
)

// CheckExecutable tests whether something is an executable. It
// examines permissions, mode, and tries to exec it directly.
func CheckExecutable(ctx context.Context, potentialBinary string, args ...string) error {
	ctx, span := traces.StartSpan(ctx)
	defer span.End()

	if err := checkExecutablePermissions(potentialBinary); err != nil {
		return err
	}

	// If we can determine that the requested executable is
	// ourself, don't try to exec. It's needless, and a potential
	// fork bomb. Ignore errors, either we get an answer or we don't.
	selfPath, _ := os.Executable()
	if filepath.Clean(selfPath) == filepath.Clean(potentialBinary) {
		return nil
	}

	// If we get ETXTBSY error when execing, this could be because this
	// binary is freshly downloaded. Retry a small number of times only
	// in that circumstance.
	// See: https://github.com/golang/go/issues/22315
	for i := 0; i < 3; i += 1 {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, potentialBinary, args...) //nolint:forbidigo // We trust the autoupdate library to find the correct location so we don't need allowedcmd

		// Set env, this should prevent launcher for fork-bombing
		cmd.Env = append(cmd.Env, "LAUNCHER_SKIP_UPDATES=TRUE")

		out, execErr := cmd.CombinedOutput()
		if execErr != nil && errors.Is(execErr, syscall.ETXTBSY) {
			continue
		}

		if ctx.Err() != nil {
			return fmt.Errorf("timeout when checking executable: %w", ctx.Err())
		}

		return supressRoutineErrors(execErr, out)
	}

	return fmt.Errorf("could not exec %s despite retries due to text file busy", potentialBinary)
}

// supressRoutineErrors attempts to tell whether the error was a
// program that has executed, and then exited, vs one that's execution
// was entirely unsuccessful. This differentiation allows us to
// detect, and recover, from corrupt updates vs something in-app.
func supressRoutineErrors(err error, combinedOutput []byte) error {
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
