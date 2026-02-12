package allowedcmd

// Package allowedcmd wraps access to exec.Cmd in order to consolidate path lookup logic.
// We mostly use hardcoded (known, safe) paths to executables, but make an exception
// to allow for looking up executable locations when it's not possible to know these
// locations in advance -- e.g. on NixOS, we cannot know the specific store path ahead
// of time. All usage of exec.Cmd in launcher should use this package.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
)

const cmdGoMaxProcs = 2

var ErrCommandNotFound = errors.New("command not found")

type AllowedCommand interface {
	//Cmd returns a exec.CommandContext suitable for subsequent usage
	Cmd(ctx context.Context, args ...string) (*TracedCmd, error)
	// Name returns the name of this allowed command
	Name() string
}

// allowedCommand is an internal struct that conforms to the AllowedCommand interface
type allowedCommand struct {
	knownPaths []string
	env        []string
}

func newAllowedCommand(knownPaths ...string) allowedCommand {
	return allowedCommand{
		knownPaths: knownPaths,
	}
}

func (ac allowedCommand) WithEnv(env string) allowedCommand {
	ac.env = append(ac.env, env)
	return ac
}

func (ac allowedCommand) Name() string {
	if len(ac.knownPaths) == 0 {
		return "~unknown~"
	}

	return ac.knownPaths[0]
}

func (ac allowedCommand) Cmd(ctx context.Context, arg ...string) (*TracedCmd, error) {
	for _, knownPath := range ac.knownPaths {
		knownPath = filepath.Clean(knownPath)

		if _, err := os.Stat(knownPath); err == nil {
			return newCmd(ctx, ac.env, knownPath, arg...), nil
		}
	}

	// Not found at known location -- return error for darwin and windows.
	// We expect to know the exact location for allowlisted commands on all
	// OSes except for a few Linux distros.
	if !allowSearchPath() {
		return nil, fmt.Errorf("%w: %s", ErrCommandNotFound, ac.Name())
	}

	for _, knownPath := range ac.knownPaths {
		cmdName := filepath.Base(knownPath)
		if foundPath, err := exec.LookPath(cmdName); err == nil {
			return newCmd(ctx, ac.env, foundPath, arg...), nil
		}
	}

	return nil, fmt.Errorf("%w: not found at %s and could not be located elsewhere", ErrCommandNotFound, ac.Name())
}

func newCmd(ctx context.Context, env []string, fullPathToCmd string, arg ...string) *TracedCmd {
	cmd := exec.CommandContext(ctx, fullPathToCmd, arg...) //nolint:forbidigo // This is our approved usage of exec.CommandContext
	cmd.Env = append(cmd.Environ(), fmt.Sprintf("GOMAXPROCS=%d", cmdGoMaxProcs))
	cmd.Env = append(cmd.Env, env...)
	return &TracedCmd{
		Ctx: ctx,
		Cmd: cmd,
	}
}

func allowSearchPath() bool {
	return IsNixOS()
}

// Save results of lookup so we don't have to stat for /etc/NIXOS every time
// we want to know.
var (
	checkedIsNixOS = &atomic.Bool{}
	isNixOS        = &atomic.Bool{}
)

func IsNixOS() bool {
	if checkedIsNixOS.Load() {
		return isNixOS.Load()
	}

	if _, err := os.Stat("/etc/NIXOS"); err == nil {
		isNixOS.Store(true)
	}

	checkedIsNixOS.Store(true)
	return isNixOS.Load()
}

// LauncherCommand is a special struct that conforms to the AllowedCommand interface. It needs it's own implementation because instead of `knownPaths`
// it uses os.Executable and resolves at call time. This handles the launcher updates.
type launcherCommand struct{}

func (_ launcherCommand) Name() string { return "launcher" }

func (_ launcherCommand)  Cmd(ctx context.Context, args ...string) (*TracedCmd, error) {
	// Try to get our current running path, this skips future path lookups
	selfPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("getting path to launcher: %w", err)
	}

// FIXME: Should we check that selfPath still exists

	envAdditions := []string{
		"LAUNCHER_SKIP_UPDATES=TRUE",
	}

	return newCmd(ctx, envAdditions, selfPath, args...), nil
}

var Launcher = launcherCommand{}
