package allowedcmd

// Package allowedcmd wraps access to exec.Cmd in order to consolidate path lookup logic.
// We mostly use hardcoded (known, safe) paths to executables, but make an exception
// to allow for looking up executable locations when it's not possible to know these
// locations in advance -- e.g. on NixOS, we cannot know the specific store path ahead
// of time. All usage of exec.Cmd in launcher should use this package.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"errors"
)

type AllowedCommand func(ctx context.Context, arg ...string) (*exec.Cmd, error)

func newCmd(ctx context.Context, fullPathToCmd string, arg ...string) *exec.Cmd {
	return exec.CommandContext(ctx, fullPathToCmd, arg...) //nolint:forbidigo // This is our approved usage of exec.CommandContext
}

var ErrCommandNotFound = errors.New("command not found")

func validatedCommand(ctx context.Context, knownPath string, arg ...string) (*exec.Cmd, error) {
	knownPath = filepath.Clean(knownPath)

	if _, err := os.Stat(knownPath); err == nil {
		return newCmd(ctx, knownPath, arg...), nil
	}

	// Not found at known location -- return error for darwin and windows.
	// We expect to know the exact location for allowlisted commands on all
	// OSes except for a few Linux distros.
	if !allowSearchPath() {
		return nil, fmt.Errorf("%w: %s", ErrCommandNotFound, knownPath)
	}

	cmdName := filepath.Base(knownPath)
	if foundPath, err := exec.LookPath(cmdName); err == nil {
		return newCmd(ctx, foundPath, arg...), nil
	}

	return nil, fmt.Errorf("%w: not found at %s and could not be located elsewhere", ErrCommandNotFound, knownPath)
}

func allowSearchPath() bool {
	return IsNixOS()
}

// Save results of lookup so we don't have to stat for /etc/NIXOS every time
// we want to know.
var (
	checkedIsNixOS = false
	isNixOS        = false
)

func IsNixOS() bool {
	if checkedIsNixOS {
		return isNixOS
	}

	if _, err := os.Stat("/etc/NIXOS"); err == nil {
		isNixOS = true
	}

	checkedIsNixOS = true
	return isNixOS
}
