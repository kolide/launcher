//go:build linux
// +build linux

package tuf

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kolide/launcher/ee/allowedcmd"
)

// patchExecutable updates the downloaded binary as necessary for it to be able to
// run on this system. On NixOS, we have to set the interpreter for any non-NixOS
// executable we want to run.
// See: https://unix.stackexchange.com/a/522823
func patchExecutable(executableLocation string) error {
	if !allowedcmd.IsNixOS() {
		return nil
	}

	interpreter, err := getInterpreter(executableLocation)
	if err != nil {
		return fmt.Errorf("getting interpreter for %s: %w", executableLocation, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd, err := allowedcmd.Patchelf(ctx, "--set-interpreter", interpreter, executableLocation)
	if err != nil {
		return fmt.Errorf("creating patchelf command: %w", err)
	}

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("running patchelf: output `%s`, error `%w`", string(out), err)
	}

	return nil
}

// getInterpreter asks patchelf what the interpreter is for the current running
// executable, assuming that's a reasonable choice given that the current executable
// is able to run.
func getInterpreter(executableLocation string) (string, error) {
	currentExecutable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("getting current running executable: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd, err := allowedcmd.Patchelf(ctx, "--print-interpreter", currentExecutable)
	if err != nil {
		return "", fmt.Errorf("creating patchelf command: %w", err)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("running patchelf: output `%s`, error `%w`", string(out), err)

	}

	return strings.TrimSpace(string(out)), nil
}
