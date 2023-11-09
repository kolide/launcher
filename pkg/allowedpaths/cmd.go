package allowedpaths

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

type AllowedCommand func(ctx context.Context, arg ...string) (*exec.Cmd, error)

func newCmd(ctx context.Context, fullPathToCmd string, arg ...string) *exec.Cmd {
	return exec.CommandContext(ctx, fullPathToCmd, arg...) //nolint:forbidigo
}

func validatedCommand(ctx context.Context, knownPath string, arg ...string) (*exec.Cmd, error) {
	knownPath = filepath.Clean(knownPath)

	if _, err := os.Stat(knownPath); err == nil {
		return newCmd(ctx, knownPath, arg...), nil
	}

	// Not found at known location -- return error for darwin and windows.
	// We expect to know the exact location for allowlisted commands on all
	// OSes except for a few Linux distros.
	if allowSearchPath() {
		return nil, fmt.Errorf("not found: %s", knownPath)
	}

	cmdName := filepath.Base(knownPath)
	if foundPath, err := exec.LookPath(cmdName); err == nil {
		return newCmd(ctx, foundPath, arg...), nil
	}

	return nil, fmt.Errorf("%s not found at %s and could not be located elsewhere", cmdName, knownPath)
}

func allowSearchPath() bool {
	if runtime.GOOS != "linux" {
		return false
	}

	// We only allow searching for binaries in PATH on NixOS
	if _, err := os.Stat("/etc/NIXOS"); err == nil {
		return true
	}

	return false
}
