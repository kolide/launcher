//go:build darwin
// +build darwin

package disclaim

/*
#cgo CFLAGS: -mmacosx-version-min=10.14 -Wall -Werror
#cgo LDFLAGS: -framework Foundation -framework Security -framework System

#include <Security/Authorization.h>
#include <Security/AuthorizationTags.h>
#include <spawn.h>
#include <sys/types.h>
#include <sys/wait.h>

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

int responsibility_spawnattrs_setdisclaim(posix_spawnattr_t attrs, int disclaim);

int spawn_disclaimed(const char *path, char *const argv[], char *const envp[]) {
    posix_spawnattr_t attrs;
    int err = posix_spawnattr_init(&attrs);
    if (err != 0) {
        return err;
    }

    // Set the disclaim attribute
    err = responsibility_spawnattrs_setdisclaim(&attrs, 1);
    if (err != 0) {
        posix_spawnattr_destroy(&attrs);
        return err;
    }

    pid_t pid;
    // passing NULL for file_actions, we capture stderr/out from running this as subcommand
    err = posix_spawn(&pid, path, NULL, &attrs, argv, envp);
    posix_spawnattr_destroy(&attrs);
    if (err != 0) {
        return err;
    }

    int status;
    if (waitpid(pid, &status, 0) == -1) {
        return -1;
    }

    // WIFEXITED will return non-zero if the child process terminated normally
    return WIFEXITED(status) ? WEXITSTATUS(status) : -1;
}
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"unsafe"

	"github.com/kolide/launcher/ee/allowedcmd"
	"github.com/kolide/launcher/pkg/log/multislogger"
)

type allowedCmdGenerator struct {
	allowedOpts map[string]struct{}
	generate    func(ctx context.Context, args ...string) (*allowedcmd.TracedCmd, error)
}

// allowedCmdGenerators maps command names to their allowed options and generator functions.
// Each entry defines which command-line options are permitted for a given command when
// running in disclaimed mode, along with the function to generate the TracedCmd.
// in most cases you should be able to set generator to the corresponding allowedcmd,
// but you may need to add a helper for additional setup if your command
// requires it (see generateBrewCommand)
var allowedCmdGenerators = map[string]allowedCmdGenerator{
	"brew": {
		allowedOpts: map[string]struct{}{
			"outdated": {},
			"--json":   {},
		},
		generate: generateBrewCommand,
	},
	"falconctl": {
		allowedOpts: map[string]struct{}{
			"stats": {},
			"-p":    {},
		},
		generate: allowedcmd.Falconctl,
	},
	"carbonblack_repcli": {
		allowedOpts: map[string]struct{}{
			"status": {},
		},
		generate: allowedcmd.Repcli,
	},
	"zscaler": {
		allowedOpts: map[string]struct{}{
			"status": {},
			"-s":     {},
			"all":    {},
		},
		generate: allowedcmd.Zscli,
	},
	"microsoft_defender_atp": {
		allowedOpts: map[string]struct{}{
			"health":   {},
			"--output": {},
			"json":     {},
		},
		generate: allowedcmd.MicrosoftDefenderATP,
	},
}

// RunDisclaimed executes a command using posix_spawn with disclaimed privileges.
// It validates the command and arguments against our allowedCmdGenerators, then spawns the process
// using C bindings to spawn_disclaimed. If the target binary is not found, it writes a
// message to stderr and returns nil to allow callers to handle missing binaries gracefully.
// An error is returned if any command validation fails, or if spawn_disclaimed is unsuccessful-
// in these cases the subcommand itself will return a non-zero exit status, meaning that the
// corresponding table Run command will see an error here as a failure to run
func RunDisclaimed(_ *multislogger.MultiSlogger, args []string) error {
	ctx := context.Background()
	cmd, err := commandToDisclaim(ctx, args)
	// this command is used to generate table data, do not error if the target binary is not found
	if err != nil && errors.Is(err, allowedcmd.ErrCommandNotFound) {
		// write that we haven't found the binary to stderr so callers can report on this if needed
		fmt.Fprint(os.Stderr, "binary is not present on device")
		return nil
	}

	if err != nil {
		return fmt.Errorf("gathering subcommand: %w", err)
	}

	cmdPath := C.CString(cmd.Path)
	defer C.free(unsafe.Pointer(cmdPath))

	// we're building up the C arguments here like []*C.char{cmdPath, arg1, arg2..., nil}
	// note that cmdPath will already be present as the first argument
	cargs := make([]*C.char, len(cmd.Args)+1) // 1 extra for null terminator
	for i, arg := range cmd.Args {
		cargs[i] = C.CString(arg)
	}
	cargs[len(cargs)-1] = nil // should be terminated by null entry

	defer func() {
		for _, arg := range cargs {
			if arg != nil {
				C.free(unsafe.Pointer(arg))
			}
		}
	}()

	cenvs := make([]*C.char, len(cmd.Environ())+1) // 1 extra for null terminator
	for i, arg := range cmd.Environ() {
		cenvs[i] = C.CString(arg)
	}
	cenvs[len(cenvs)-1] = nil // should be terminated by null entry

	defer func() {
		for _, arg := range cenvs {
			if arg != nil {
				C.free(unsafe.Pointer(arg))
			}
		}
	}()

	ret := C.spawn_disclaimed(cmdPath, &cargs[0], &cenvs[0])
	if int(ret) != 0 {
		return fmt.Errorf("spawn_disclaimed returned error code %d", int(ret))
	}

	return nil
}

func commandToDisclaim(ctx context.Context, args []string) (*allowedcmd.TracedCmd, error) {
	if len(args) < 1 {
		return nil, errors.New("rundisclaimed expects at least 1 subcommand")
	}

	subcommand := args[0]
	cmdArgs := args[1:]
	generator, err := getCmdGenerator(subcommand)
	if err != nil {
		return nil, fmt.Errorf("validating command: %w", err)
	}

	for _, arg := range cmdArgs {
		if _, ok := generator.allowedOpts[arg]; !ok {
			return nil, fmt.Errorf("invalid argument provided for '%s' command: '%s'", subcommand, arg)
		}
	}

	return generator.generate(ctx, cmdArgs...)
}

func getCmdGenerator(cmd string) (*allowedCmdGenerator, error) {
	if generator, ok := allowedCmdGenerators[cmd]; ok {
		return &generator, nil
	}

	return nil, fmt.Errorf("unsupported command '%s' for rundisclaimed", cmd)
}

func generateBrewCommand(ctx context.Context, args ...string) (*allowedcmd.TracedCmd, error) {
	cmd, err := allowedcmd.Brew(ctx, args...)
	if err != nil {
		return nil, err
	}

	// we should already be running as the intended user at this point,
	// (we cannot set UID/GID with posix_spawn). but we will need to explicitly
	// add PWD and HOME env variables at this level for spawndisclaimed
	currentUser, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("getting current user: %w", err)
	}

	cmd.Env = append(cmd.Environ(), "PWD="+currentUser.HomeDir, "HOME="+currentUser.HomeDir)
	return cmd, nil
}
