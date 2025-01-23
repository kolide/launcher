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

	"github.com/kolide/launcher/pkg/traces"
)

type AllowedCommand func(ctx context.Context, arg ...string) (*TracedCmd, error)

type TracedCmd struct {
	ctx context.Context // nolint:structcheck // This is an approved usage of context for short lived cmd
	*exec.Cmd
}

// Start overrides the Start method to add tracing before executing the command.
func (t *TracedCmd) Start() error {
	_, span := traces.StartSpan(t.ctx, "path", t.Cmd.Path, "args", fmt.Sprintf("%+v", t.Cmd.Args))
	defer span.End()

	return t.Cmd.Start()
}

// Run overrides the Run method to add tracing before running the command.
func (t *TracedCmd) Run() error {
	_, span := traces.StartSpan(t.ctx, "path", t.Cmd.Path, "args", fmt.Sprintf("%+v", t.Cmd.Args))
	defer span.End()

	return t.Cmd.Run()
}

// Output overrides the Output method to add tracing before capturing output.
func (t *TracedCmd) Output() ([]byte, error) {
	_, span := traces.StartSpan(t.ctx, "path", t.Cmd.Path, "args", fmt.Sprintf("%+v", t.Cmd.Args))
	defer span.End()

	return t.Cmd.Output()
}

// CombinedOutput overrides the CombinedOutput method to add tracing before capturing combined output.
func (t *TracedCmd) CombinedOutput() ([]byte, error) {
	_, span := traces.StartSpan(t.ctx, "path", t.Cmd.Path, "args", fmt.Sprintf("%+v", t.Cmd.Args))
	defer span.End()

	return t.Cmd.CombinedOutput()
}

func newCmd(ctx context.Context, fullPathToCmd string, arg ...string) *TracedCmd {
	return &TracedCmd{
		ctx: ctx,
		Cmd: exec.CommandContext(ctx, fullPathToCmd, arg...), //nolint:forbidigo // This is our approved usage of exec.CommandContext
	}
}

var ErrCommandNotFound = errors.New("command not found")

func validatedCommand(ctx context.Context, knownPath string, arg ...string) (*TracedCmd, error) {
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
