package tuf

import (
	"context"
	"fmt"
	"path/filepath"
	"time"
)

func patchExecutable(executableLocation string) error {
	if !allowedcmd.IsNixOS() {
		return nil
	}

	interpreter, err := getLoader(executableLocation)
	if err != nil {
		return fmt.Errorf("getting loader for %s: %w", executableLocation, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := allowedcmd.Patchelf(ctx, "--set-interpreter", interpreter, executableLocation)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("running patchelf: output `%s`, error `%w`", string(out), err)
	}

	return nil
}

func getLoader(_ string) (string, error) {
	matches, err := filepath.Glob("/nix/store/*glibc*/lib/ld-linux-x86-64.so.2")
	if err != nil {
		return "", fmt.Errorf("globbing for loader: %w", err)
	}

	return matches[0], nil
}
