//go:build darwin
// +build darwin

package allowedcmd

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

// see https://www.qt.io/blog/the-curious-case-of-the-responsible-process for a wonderful writeup
// of how this works
int responsibility_spawnattrs_setdisclaim(posix_spawnattr_t attrs, int disclaim);

int spawn_disclaimed(const char *path, char *const argv[], char *const envp[], char **stdout, char **stderr) {
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

    // set up the stdout/stderr redirects
    int pipefd_out[2];
    int pipefd_err[2];
    if (pipe(pipefd_out) || pipe(pipefd_err)) {
        posix_spawnattr_destroy(&attrs);
        return -1;
    }

    posix_spawn_file_actions_t file_actions;
    posix_spawn_file_actions_init(&file_actions);
    posix_spawn_file_actions_adddup2(&file_actions, pipefd_out[1], STDOUT_FILENO);
    posix_spawn_file_actions_adddup2(&file_actions, pipefd_err[1], STDERR_FILENO);
    posix_spawn_file_actions_addclose(&file_actions, pipefd_out[0]);
    posix_spawn_file_actions_addclose(&file_actions, pipefd_err[0]);
    posix_spawn_file_actions_addclose(&file_actions, pipefd_out[1]);
    posix_spawn_file_actions_addclose(&file_actions, pipefd_err[1]);

    pid_t pid;
    err = posix_spawn(&pid, path, &file_actions, &attrs, argv, envp);
    posix_spawnattr_destroy(&attrs);
    posix_spawn_file_actions_destroy(&file_actions);
    close(pipefd_out[1]);  // Close write ends of pipes in parent
    close(pipefd_err[1]);

    if (err != 0) {
        close(pipefd_out[0]); // close read ends
        close(pipefd_err[0]);
        return err;
    }

    char buffer[1024];
    size_t out_size = 0, err_size = 0;

    // Read stdout
    FILE *out_fp = fdopen(pipefd_out[0], "r");
    if (out_fp) {
        while (fgets(buffer, sizeof(buffer), out_fp)) {
            size_t len = strlen(buffer);
            *stdout = realloc(*stdout, out_size + len + 1);
            memcpy(*stdout + out_size, buffer, len + 1);
            out_size += len;
        }
        fclose(out_fp);
    }

    // Read stderr
    FILE *err_fp = fdopen(pipefd_err[0], "r");
    if (err_fp) {
        while (fgets(buffer, sizeof(buffer), err_fp)) {
            size_t len = strlen(buffer);
            *stderr = realloc(*stderr, err_size + len + 1);
            memcpy(*stderr + err_size, buffer, len + 1);
            err_size += len;
        }
        fclose(err_fp);
    }

    int status;
    if (waitpid(pid, &status, 0) == -1) {
        close(pipefd_out[0]);  // close read ends
        close(pipefd_err[0]);
        return -1;
    }

   close(pipefd_out[0]);  // close read ends
   close(pipefd_err[0]);

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

	"github.com/kolide/launcher/pkg/log/multislogger"
)

func RunDisclaimed(_ *multislogger.MultiSlogger, args []string) error {
	ctx := context.Background()
	cmd, err := commandToDisclaim(ctx, args)
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

	var output, stderr *C.char

	C.spawn_disclaimed(cmdPath, &cargs[0], &cenvs[0], &output, &stderr)

	goOut := C.GoString(output)
	goErr := C.GoString(stderr)

	C.free(unsafe.Pointer(output))
	C.free(unsafe.Pointer(stderr))

	goBytes := []byte(goOut + goErr)

	if _, err := os.Stdout.Write(goBytes); err != nil {
		return fmt.Errorf("writing results to stdout: %w", err)
	}

	return nil
}

func commandToDisclaim(ctx context.Context, args []string) (*TracedCmd, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("rundisclaimed expects 1 subcommand, received args of length %d", len(args))
	}

	subcommand := args[0]
	switch subcommand {
	case "brew":
		return generateBrewCommand(ctx)
	default:
		return nil, errors.New("unsupported command for rundisclaimed")
	}
}

func generateBrewCommand(ctx context.Context) (*TracedCmd, error) {
	cmd, err := Brew(ctx, "outdated", "--json")
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
