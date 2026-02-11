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

	"github.com/kolide/launcher/ee/observability"
)

const cmdGoMaxProcs = 2

type TracedCmd struct {
	Ctx context.Context // nolint:containedctx // This is an approved usage of context for short lived cmd
	*exec.Cmd
}

// Start overrides the Start method to add tracing before executing the command.
func (t *TracedCmd) Start() error {
	_, span := observability.StartSpan(t.Ctx, "path", t.Path, "args", fmt.Sprintf("%+v", t.Args))
	defer span.End()

	return t.Cmd.Start() //nolint:forbidigo // This is our approved usage of t.Cmd.Start()
}

func (t *TracedCmd) String() string {
	return fmt.Sprintf("%+v", t.Args)
}

// Run overrides the Run method to add tracing before running the command.
func (t *TracedCmd) Run() error {
	_, span := observability.StartSpan(t.Ctx, "path", t.Path, "args", fmt.Sprintf("%+v", t.Args))
	defer span.End()

	return t.Cmd.Run() //nolint:forbidigo // This is our approved usage of t.Cmd.Run()
}

// Output overrides the Output method to add tracing before capturing output.
func (t *TracedCmd) Output() ([]byte, error) {
	_, span := observability.StartSpan(t.Ctx, "path", t.Path, "args", fmt.Sprintf("%+v", t.Args))
	defer span.End()

	return t.Cmd.Output() //nolint:forbidigo // This is our approved usage of t.Cmd.Output()
}

// CombinedOutput overrides the CombinedOutput method to add tracing before capturing combined output.
func (t *TracedCmd) CombinedOutput() ([]byte, error) {
	_, span := observability.StartSpan(t.Ctx, "path", t.Path, "args", fmt.Sprintf("%+v", t.Args))
	defer span.End()

	return t.Cmd.CombinedOutput() //nolint:forbidigo // This is our approved usage of t.Cmd.CombinedOutput()
}

var ErrCommandNotFound = errors.New("command not found")

type AllowedCommand struct {
	knownPaths []string
	env        []string
}

func newAllowedCommand(knownPaths ...string) AllowedCommand {
	return AllowedCommand{
		knownPaths: knownPaths,
	}
}

func (ac AllowedCommand) WithEnv(env string) AllowedCommand {
	ac.env = append(ac.env, env)
	return ac
}

func (ac AllowedCommand) Name() string {
	if len(ac.knownPaths) == 0 {
		return "~unknown~"
	}

	return ac.knownPaths[0]
}

func (ac AllowedCommand) Cmd(ctx context.Context, arg ...string) (*TracedCmd, error) {
	for _, knownPath := range ac.knownPaths {
		knownPath = filepath.Clean(knownPath)

		if _, err := os.Stat(knownPath); err == nil {
			return ac.newCmd(ctx, knownPath, arg...), nil
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
			return ac.newCmd(ctx, foundPath, arg...), nil
		}
	}

	return nil, fmt.Errorf("%w: not found at %s and could not be located elsewhere", ErrCommandNotFound, ac.Name())
}

func (ac AllowedCommand) newCmd(ctx context.Context, fullPathToCmd string, arg ...string) *TracedCmd {
	cmd := exec.CommandContext(ctx, fullPathToCmd, arg...) //nolint:forbidigo // This is our approved usage of exec.CommandContext
	cmd.Env = append(cmd.Environ(), fmt.Sprintf("GOMAXPROCS=%d", cmdGoMaxProcs))
	cmd.Env = append(cmd.Env, ac.env...)
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

var Launcher = func() AllowedCommand {
	// Try to get our current running path, this skips future path lookups
	//
	// But, not sure what we should do in the face of an error. This is an initializer,
	// so there's not a great way to pass it along
	selfPath, err := os.Executable()
	if err != nil {
		//return nil, fmt.Errorf("getting path to launcher: %w", err)
		selfPath = ""
	}

	return newAllowedCommand(selfPath).WithEnv("LAUNCHER_SKIP_UPDATES=TRUE")
}()
