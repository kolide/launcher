package allowedpaths

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// CommandWithLookup should be used when the full path to the command is not known and it is
// likely to be found in PATH.
func CommandWithLookup(name string, arg ...string) (*exec.Cmd, error) {
	fullPathToCommand, err := exec.LookPath(name)
	if err != nil {
		return nil, fmt.Errorf("looking up path to %s: %w", name, err)
	}

	return CommandWithPath(fullPathToCommand, arg...)
}

// CommandWithPath should be used when the full path to the command is known, or unlikely to be
// found in PATH.
func CommandWithPath(fullPathToCommand string, arg ...string) (*exec.Cmd, error) {
	fullPathToCommand = filepath.Clean(fullPathToCommand)

	if err := pathIsAllowed(fullPathToCommand); err != nil {
		return nil, fmt.Errorf("path is not allowed: %w", err)
	}

	return exec.Command(fullPathToCommand, arg...), nil
}

// CommandContextWithLookup should be used when the full path to the command is not known and it is
// likely to be found in PATH.
func CommandContextWithLookup(ctx context.Context, name string, arg ...string) (*exec.Cmd, error) {
	fullPathToCommand, err := exec.LookPath(name)
	if err != nil {
		return nil, fmt.Errorf("looking up path to %s: %w", name, err)
	}

	return CommandContextWithPath(ctx, fullPathToCommand, arg...)
}

// CommandContextWithPath should be used when the full path to the command is known, or unlikely to be
// found in PATH.
func CommandContextWithPath(ctx context.Context, fullPathToCommand string, arg ...string) (*exec.Cmd, error) {
	fullPathToCommand = filepath.Clean(fullPathToCommand)

	if err := pathIsAllowed(fullPathToCommand); err != nil {
		return nil, fmt.Errorf("path is not allowed: %w", err)
	}

	return exec.CommandContext(ctx, fullPathToCommand, arg...), nil
}

// pathIsAllowed validates the path to the command against our allowlist.
func pathIsAllowed(fullPathToCommand string) error {
	cmdName := strings.ToLower(filepath.Base(fullPathToCommand))

	// We trust the autoupdate libraries to select the correct paths
	if cmdName == "launcher" || cmdName == "launcher.exe" || cmdName == "osqueryd" || cmdName == "osqueryd.exe" {
		return nil
	}

	// Check that we have known paths for the given command
	knownCmdPaths, ok := knownPaths[cmdName]
	if !ok {
		return fmt.Errorf("no known paths for command %s, cannot validate path %s", cmdName, fullPathToCommand)
	}

	// Check if this path is registered
	if _, ok := knownCmdPaths[fullPathToCommand]; ok {
		return nil
	}

	// Check to make sure the command is at least in a known directory
	for _, knownPathPrefix := range knownPathPrefixes {
		if strings.HasPrefix(fullPathToCommand, knownPathPrefix) {
			return nil
		}
	}

	return fmt.Errorf("%s not in known paths and does not have known prefix", fullPathToCommand)
}
